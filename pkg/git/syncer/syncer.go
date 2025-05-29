package syncer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	giturls "github.com/clbiggs/git-sync/pkg/git/git-urls"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/transport"
	httpgit "github.com/go-git/go-git/v5/plumbing/transport/http"
	gossh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

type AuthOptions struct {
	Repo              string
	Username          string
	Password          string
	PasswordFile      string
	SSHPrivateKeyFile string
	InsecureSkipTLS   bool
	KnownHostsFile    string
}

type SyncOptions struct {
	Path         string
	RefName      plumbing.ReferenceName
	CABuntleFile string
	PollInterval time.Duration
	Auth         AuthOptions
}

type SyncStatus struct {
	LastChecked time.Time `json:"last_checked"`
	LastUpdated time.Time `json:"last_updated"`
	LatestHash  string    `json:"latest_commit"`
}

type Syncer struct {
	Options       SyncOptions
	status        SyncStatus
	statusLock    sync.Mutex
	pollingCtx    context.Context
	pollingCancel context.CancelFunc
}

func NewSyncer(options SyncOptions) *Syncer {
	return &Syncer{
		Options:    options,
		status:     SyncStatus{},
		statusLock: sync.Mutex{},
	}
}

func (s *Syncer) Status() SyncStatus {
	s.statusLock.Lock()
	defer s.statusLock.Unlock()
	return s.status
}

func (s *Syncer) Start() {
	if s.pollingCtx != nil && s.pollingCtx.Err() == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	s.pollingCtx = ctx
	s.pollingCancel = cancel

	go s.startPolling(ctx)
}

func (s *Syncer) Stop() {
	s.pollingCancel()
	s.pollingCtx = nil
	s.pollingCancel = nil
}

func (s *Syncer) startPolling(ctx context.Context) {
	ticker := time.NewTicker(s.Options.PollInterval)
	defer ticker.Stop()

	log.Printf("Starting Polling on Repo: %s", s.Options.Auth.Repo)

	for {
		select {
		case <-ticker.C:
			err := s.syncRepo(ctx, false)
			if err != nil {
				log.Printf("Error Syncing Repo: %s\n%v", s.Options.Auth.Repo, err)
			}
		case <-ctx.Done():
			log.Printf("Stopping Polling on Repo: %s", s.Options.Auth.Repo)
			return
		}
	}
}

func (s *Syncer) ForceSync() error {
	return s.syncRepo(context.Background(), true)
}

func (s *Syncer) syncRepo(ctx context.Context, forcePull bool) error {
	transport.UnsupportedCapabilities = []capability.Capability{
		capability.ThinPack,
	}

	s.statusLock.Lock()
	defer s.statusLock.Unlock()

	s.status.LastChecked = time.Now()

	var repo *git.Repository
	var err error

	log.Println("Looking for Repo locally...")
	repo, err = openRepo(s.Options)

	switch {
	case errors.Is(err, git.ErrRepositoryNotExists) || os.IsNotExist(err):
		log.Println("Repo not found, Clonning...")
		repo, err = cloneRepo(ctx, s.Options)
		if err != nil {
			return fmt.Errorf("clone failed: %w", err)
		}

		log.Println("Clonning Completed.")
	case err != nil:
		return fmt.Errorf("failed to open repo: %w", err)
	default:
		// if repo already exists, make sure the target branch hasn't changed.
		err = switchReference(ctx, repo, s.Options)
		if err != nil {
			return fmt.Errorf("failed to switch branch: %w", err)
		}
	}

	log.Println("Fetching Repo...")
	err = fetchRepo(ctx, repo, s.Options)
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch failed: %w", err)
	}
	log.Println("Fetch Completed.")

	ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", s.Options.RefName.Short()), true)
	if err != nil {
		return fmt.Errorf("reference error: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	hash := ref.Hash().String()
	if forcePull || hash != s.status.LatestHash {
		log.Println("Updating repo to latest commit", hash)
		err = pullRepo(ctx, w, s.Options)
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return fmt.Errorf("pull failed: %w", err)
		}
		s.status.LatestHash = hash
		s.status.LastUpdated = time.Now()
		log.Println("Update Completed.")
	} else {
		log.Println("No changes.")

		// Manually reset the worktree to match the latest commit fully
		// This is to handle any cases where local did not complete extract, but git commit is pulled
		err = w.Reset(&git.ResetOptions{
			Mode:   git.HardReset,
			Commit: ref.Hash(),
		})
		if err != nil {
			return fmt.Errorf("reset branch failed: %w", err)
		}
	}

	return nil
}

func switchReference(ctx context.Context, repo *git.Repository, opts SyncOptions) error {
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD reference: %w", err)
	}

	currentRef := headRef.Name().String()
	if currentRef != opts.RefName.String() {
		log.Printf("Switching from reference %s to %s", currentRef, opts.RefName.String())

		log.Println("Fetching Repo to get remote references...")
		err = fetchRepo(ctx, repo, opts)
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return fmt.Errorf("fetch failed: %w", err)
		}
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			log.Println("Already up to date.")
		}
		log.Println("Fetch Completed.")

		w, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}

		remoteRefName := plumbing.NewRemoteReferenceName("origin", opts.RefName.Short())
		//		localRefName := plumbing.NewBranchReferenceName(opts.RefName.Short())
		//
		log.Printf("Checking out %s", remoteRefName.String())
		//		remoteRef, err := repo.Reference(remoteRefName, true)
		//		if err != nil {
		//			return fmt.Errorf("could not find remote branch %s: %w", remoteRefName, err)
		//		}
		//
		//		localRef := plumbing.NewHashReference(localRefName, remoteRef.Hash())
		//
		//		if err = repo.Storer.SetReference(localRef); err != nil {
		//			return fmt.Errorf("failed to create local branch: %w", err)
		//		}
		//
		//		err = w.Checkout(&git.CheckoutOptions{
		//			Branch: localRefName,
		//			Force:  true,
		//		})

		err = w.Checkout(&git.CheckoutOptions{
			Branch: remoteRefName,
			Force:  true,
		})
		if err != nil {
			return fmt.Errorf("checkout failed: %w", err)
		}

		err = removeOldBranches(repo)
		if err != nil {
			return fmt.Errorf("deleting old branches failed: %w", err)
		}
	}
	return nil
}

func removeOldBranches(repo *git.Repository) error {
	log.Println("Removing unused branches...")
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	currentBranch := headRef.Name()

	branches, err := repo.Branches()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	err = branches.ForEach(func(ref *plumbing.Reference) error {
		branchName := ref.Name()
		if branchName != currentBranch {
			err1 := repo.Storer.RemoveReference(ref.Name())
			if err1 != nil {
				return fmt.Errorf("failed to delete branch %s: %w", branchName, err1)
			}
			fmt.Printf("Deleted branch: %s\n", branchName)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func cloneRepo(ctx context.Context, opts SyncOptions) (*git.Repository, error) {
	log.Println("Cloning repository...")

	auth, err := createAuthFromOpts(opts.Auth)
	if err != nil {
		return nil, err
	}

	caBundle, err := getCABundleFromFile(opts.CABuntleFile)
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainCloneContext(ctx, opts.Path, false, &git.CloneOptions{
		URL:             opts.Auth.Repo,
		ReferenceName:   opts.RefName,
		SingleBranch:    true,
		Auth:            auth,
		InsecureSkipTLS: opts.Auth.InsecureSkipTLS,
		CABundle:        caBundle,
	})
	if err != nil {
		return nil, err
	}

	return repo, nil
}

func openRepo(opts SyncOptions) (*git.Repository, error) {
	repo, err := git.PlainOpen(opts.Path)
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func fetchRepo(ctx context.Context, repo *git.Repository, opts SyncOptions) error {
	auth, err := createAuthFromOpts(opts.Auth)
	if err != nil {
		return err
	}

	caBundle, err := getCABundleFromFile(opts.CABuntleFile)
	if err != nil {
		return err
	}

	err = repo.FetchContext(ctx, &git.FetchOptions{
		RemoteName:      "origin",
		Auth:            auth,
		Force:           true,
		InsecureSkipTLS: opts.Auth.InsecureSkipTLS,
		Tags:            git.AllTags,
		Prune:           true,
		CABundle:        caBundle,
		RefSpecs:        []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*"},
	})

	return err
}

func pullRepo(ctx context.Context, worktree *git.Worktree, opts SyncOptions) error {
	auth, err := createAuthFromOpts(opts.Auth)
	if err != nil {
		return err
	}

	caBundle, err := getCABundleFromFile(opts.CABuntleFile)
	if err != nil {
		return err
	}

	err = worktree.PullContext(ctx, &git.PullOptions{
		RemoteName:      "origin",
		SingleBranch:    false,
		Auth:            auth,
		Force:           true,
		InsecureSkipTLS: opts.Auth.InsecureSkipTLS,
		ReferenceName:   opts.RefName,
		CABundle:        caBundle,
	})

	return err
}

func getCABundleFromFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}

	log.Printf("Using CABundle: %s", path)
	return os.ReadFile(path)
}

func createAuthFromOpts(opts AuthOptions) (transport.AuthMethod, error) {
	gitURL, err := giturls.Parse(opts.Repo)
	if err != nil {
		return nil, err
	}

	isSSH := gitURL.Scheme == "ssh"

	if isSSH {
		if opts.SSHPrivateKeyFile != "" {
			privateKey, err := os.ReadFile(opts.SSHPrivateKeyFile)
			if err != nil {
				return nil, err
			}
			auth, err := gossh.NewPublicKeys(gitURL.User.Username(), privateKey, "")
			if err != nil {
				return nil, err
			}
			if opts.KnownHostsFile != "" {
				knownHosts, err := os.ReadFile(opts.KnownHostsFile)
				if err != nil {
					return nil, err
				}
				knownHostsCallBack, err := createKnownHostsCallBack(knownHosts)
				if err != nil {
					return nil, err
				}
				auth.HostKeyCallback = knownHostsCallBack
			} else {
				auth.HostKeyCallback = ssh.InsecureIgnoreHostKey() //nolint:gosec // dev support
			}
			return auth, nil
		} else if opts.Password != "" {
			return &gossh.Password{
				User:     opts.Username,
				Password: opts.Password,
			}, nil
		}
	}

	if opts.Username != "" {
		var password string
		if opts.PasswordFile != "" {
			tmpPass, err := os.ReadFile(opts.PasswordFile)
			if err != nil {
				return nil, err
			}
			password = string(tmpPass)
		} else if opts.Password != "" {
			password = opts.Password
		}

		return &httpgit.BasicAuth{
			Username: opts.Username,
			Password: password,
		}, nil
	}

	return nil, nil
}

func createKnownHostsCallBack(knownHosts []byte) (ssh.HostKeyCallback, error) {
	f, err := os.CreateTemp("", "known_hosts")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(f.Name())
	defer f.Close()

	if _, err := f.Write(knownHosts); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("closing knownHosts file %s: %w", f.Name(), err)
	}

	return gossh.NewKnownHostsCallback(f.Name())
}
