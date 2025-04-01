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
	"github.com/go-git/go-git/v5/plumbing"
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
	Branch       string
	CABuntleFile string
	PollInterval time.Duration
	Auth         AuthOptions
}

type SyncStatus struct {
	LastChecked time.Time `json:"last_checked"`
	LastUpdated time.Time `json:"last_updated"`
	LatestHash  string    `json:"latest_commit"`
	Cloned      bool      `json:"cloned"`
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
	s.statusLock.Lock()
	defer s.statusLock.Unlock()

	s.status.LastChecked = time.Now()

	var repo *git.Repository
	var err error

	repo, err = openRepo(s.Options)

	if errors.Is(err, git.ErrRepositoryNotExists) || os.IsNotExist(err) {
		repo, err = cloneRepo(ctx, s.Options)
		if err != nil {
			return fmt.Errorf("clone failed: %w", err)
		}

		s.status.Cloned = true
	} else if err != nil {
		return fmt.Errorf("failed to open repo: %w", err)
	}

	err = fetchRepo(ctx, repo, s.Options)
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch failed: %w", err)
	}

	ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", s.Options.Branch), true)
	if err != nil {
		return fmt.Errorf("reference error: %w", err)
	}

	hash := ref.Hash().String()
	if forcePull || hash != s.status.LatestHash {
		w, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}
		log.Println("Updating repo to latest commit", hash)
		err = pullRepo(ctx, w, s.Options)
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return fmt.Errorf("pull failed: %w", err)
		}
		s.status.LatestHash = hash
		s.status.LastUpdated = time.Now()
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
		ReferenceName:   plumbing.NewBranchReferenceName(opts.Branch),
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
		SingleBranch:    true,
		Auth:            auth,
		Force:           true,
		InsecureSkipTLS: opts.Auth.InsecureSkipTLS,
		ReferenceName:   plumbing.NewBranchReferenceName(opts.Branch),
		CABundle:        caBundle,
	})

	return err
}

func getCABundleFromFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
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
