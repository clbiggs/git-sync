package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/clbiggs/git-sync/internal/handlers"
	"github.com/clbiggs/git-sync/internal/middleware"
	"github.com/clbiggs/git-sync/pkg/git/syncer"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/gorilla/mux"
)

type Configuration struct {
	Repo                string
	Path                string
	RefName             string
	CABuntleFile        string
	Username            string
	Password            string
	PasswordFile        string
	SSHPrivateKeyFile   string
	InsecureSkipTLS     bool
	KnownHostsFile      string
	PollInterval        time.Duration
	EnableWebhook       bool
	WebhookUsername     string
	WebhookPassword     string
	WebhookPasswordFile string
	ServerAddr          string
}

const (
	DefaultInterval            = 900
	DefaultServerAddr          = ":8080"
	DefaultServerHeaderTimeout = 3
)

var config Configuration

func main() {
	loadConfigFromFlagsOrEnv()
	validateConfig()

	sync := syncer.NewSyncer(syncer.SyncOptions{
		Path:         config.Path,
		RefName:      plumbing.ReferenceName(config.RefName),
		CABuntleFile: config.CABuntleFile,
		PollInterval: config.PollInterval,
		Auth: syncer.AuthOptions{
			Repo:              config.Repo,
			Username:          config.Username,
			Password:          config.Password,
			PasswordFile:      config.PasswordFile,
			SSHPrivateKeyFile: config.SSHPrivateKeyFile,
			InsecureSkipTLS:   config.InsecureSkipTLS,
			KnownHostsFile:    config.KnownHostsFile,
		},
	})

	// Perform initial sync
	log.Printf("Performing Initial Sync...: %s", sync.Options.Auth.Repo)
	err := sync.ForceSync()
	if err != nil {
		log.Printf("failed initial sync: %v", err)

		log.Println("Deleting local files and attempting pull...")
		err = os.RemoveAll(config.Path)
		if err != nil {
			log.Fatalf("Error deleting local files: %v", err)
		}

		err = sync.ForceSync()
		if err != nil {
			log.Fatalf("failed to re-clone repo: %v", err)
		}
	}
	log.Println("Initial Sync Completed.")

	// Start polling and syncing repo.
	sync.Start()

	server, err := setupHTTPServer(config.ServerAddr, sync)
	if err != nil {
		log.Fatalf("Error building router: %v", err)
	}

	log.Printf("Server started on %s", config.ServerAddr)
	log.Fatal(server.ListenAndServe())
}

func setupHTTPServer(serverAddress string, sync *syncer.Syncer) (*http.Server, error) {
	router, err := setupRouter(sync)
	if err != nil {
		return nil, err
	}

	return &http.Server{
		Addr:              serverAddress,
		ReadHeaderTimeout: DefaultServerHeaderTimeout * time.Second,
		Handler:           router,
	}, nil
}

func setupRouter(sync *syncer.Syncer) (*mux.Router, error) {
	router := mux.NewRouter()
	var password string
	if config.WebhookPasswordFile != "" {
		tmpPass, err := os.ReadFile(config.WebhookPasswordFile)
		if err != nil {
			return nil, err
		}
		password = string(tmpPass)
	} else if config.WebhookPassword != "" {
		password = config.WebhookPassword
	}

	if config.EnableWebhook {
		router.HandleFunc("/webhook", middleware.BasicAuthMiddleware(handlers.WebhookHandler(sync), config.WebhookUsername, password)).Methods("POST")
	}
	router.HandleFunc("/status", handlers.StatusHandler(sync)).Methods("GET")
	router.HandleFunc("/liveness", handlers.LivenessHandler()).Methods("GET")

	return router, nil
}

func loadConfigFromFlagsOrEnv() {
	repo := flag.String("repo", os.Getenv("GIT_REPO"), "Git repo URL")
	path := flag.String("path", os.Getenv("TARGET_PATH"), "Local repo path")
	branch := flag.String("branch", getEnv("BRANCH", "main"), "Branch to track. The <ref> argument takes precident over this.")
	ref := flag.String("ref", os.Getenv("REF_NAME"), "Reference name. Use the refs/heads/main or refs/tags/v1.0.0 format.")
	bundleFile := flag.String("ca-bundle-file", os.Getenv("CA_BUNDLE"), "CA Certificate bundle file path")
	interval := flag.Duration("interval", getEnvDuration("POLL_INTERVAL", DefaultInterval*time.Second), "Polling interval")
	username := flag.String("username", os.Getenv("GIT_USERNAME"), "Git username/token")
	password := flag.String("password", os.Getenv("GIT_PASSWORD"), "Git password/token")
	passwordFile := flag.String("password-file", os.Getenv("GIT_PASSWORD_FILE"), "Path to file containing Git password/token")
	sshFile := flag.String("ssh-key-file", os.Getenv("GIT_SSHKEY_FILE"), "Path to file containing Git SSH Private key")
	insecure := flag.Bool("insecure", getEnvBool("INSECURE_TLS", false), "Use insecure TLS connection")
	knownHosts := flag.String("known-hosts-file", os.Getenv("KNOWN_HOSTS_FILE"), "Path to file containing known hosts")
	enableWebhook := flag.Bool("webhook-enabled", getEnvBool("WEBHOOK_ENABLED", true), "Enable/Disble the webhook api. Default: true")
	webUsername := flag.String("webhook-username", os.Getenv("WEBHOOK_USERNAME"), "Webhook basic auth user")
	webPassword := flag.String("webhook-password", os.Getenv("WEBHOOK_PASSWORD"), "Webhook basic auth password")
	webPasswordFile := flag.String("webhook-password-file", os.Getenv("WEBHOOK_PASSWORD_FILE"), "Webhook basic auth password file path")
	serverAddr := flag.String("server-address", getEnv("SERVER_ADDRESS", DefaultServerAddr), "Webhook server address")

	flag.Parse()

	refname := *ref
	if refname == "" {
		refname = "refs/heads/" + *branch
	}

	config = Configuration{
		Repo:                *repo,
		Path:                *path,
		RefName:             refname,
		CABuntleFile:        *bundleFile,
		Username:            *username,
		Password:            *password,
		PasswordFile:        *passwordFile,
		SSHPrivateKeyFile:   *sshFile,
		InsecureSkipTLS:     *insecure,
		KnownHostsFile:      *knownHosts,
		PollInterval:        *interval,
		EnableWebhook:       *enableWebhook,
		WebhookUsername:     *webUsername,
		WebhookPassword:     *webPassword,
		WebhookPasswordFile: *webPasswordFile,
		ServerAddr:          *serverAddr,
	}
}

func validateConfig() {
	missing := []string{}
	if config.Repo == "" {
		missing = append(missing, "repo")
	}
	if config.Path == "" {
		missing = append(missing, "path")
	}

	if len(missing) > 0 {
		log.Fatalf("Missing required parameters: %s", strings.Join(missing, ", "))
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}

	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return false
	}
	return val
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		dur, err := time.ParseDuration(val)
		if err == nil {
			return dur
		}
		log.Printf("Invalid duration for %s: %s", key, val)
	}
	return fallback
}
