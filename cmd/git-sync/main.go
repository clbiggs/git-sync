package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"
)

type Configuration struct {
	Repo                string
	Path                string
	Branch              string
	CABuntleFile        string
	Username            string
	Password            string
	PasswordFile        string
	SSHPrivateKeyFile   string
	InsecureSkipTLS     bool
	KnownHostsFile      string
	PollInterval        time.Duration
	WebhookUsername     string
	WebhookPassword     string
	WebhookPasswordFile string
	ServerAddr          string
}

const (
	DefaultInterval   = 900
	DefaultServerAddr = ":8080"
)

var config Configuration

func main() {
	loadConfigFromFlagsOrEnv()

	log.Printf("Repo: %s", config.Repo)
}

func loadConfigFromFlagsOrEnv() {
	repo := flag.String("repo", os.Getenv("GIT_REPO"), "Git repo URL")
	path := flag.String("path", os.Getenv("TARGET_PATH"), "Local repo path")
	branch := flag.String("branch", getEnv("BRANCH", "main"), "Branch to track")
	bundleFile := flag.String("ca-bundle-file", os.Getenv("CA_BUNDLE"), "CA Certificate bundle file path")
	interval := flag.Duration("interval", getEnvDuration("POLL_INTERVAL", DefaultInterval*time.Second), "Polling interval")
	username := flag.String("username", os.Getenv("GIT_USERNAME"), "Git username/token")
	password := flag.String("password", os.Getenv("GIT_PASSWORD"), "Git password/token")
	passwordFile := flag.String("password-file", os.Getenv("GIT_PASSWORD_FILE"), "Path to file containing Git password/token")
	sshFile := flag.String("ssh-key-file", os.Getenv("GIT_SSHKEY_FILE"), "Path to file containing Git SSH Private key")
	insecure := flag.Bool("insecure", getEnvBool("INSECURE_TLS", false), "Use insecure TLS connection")
	knownHosts := flag.String("known-hosts-file", os.Getenv("KNOWN_HOSTS_FILE"), "Path to file containing known hosts")
	webUsername := flag.String("webhook-username", os.Getenv("WEBHOOK_USERNAME"), "Webhook basic auth user")
	webPassword := flag.String("webhook-password", os.Getenv("WEBHOOK_PASSWORD"), "Webhook basic auth password")
	webPasswordFile := flag.String("webhook-password-file", os.Getenv("WEBHOOK_PASSWORD_FILE"), "Webhook basic auth password file path")
	serverAddr := flag.String("server-address", getEnv("SERVER_ADDRESS", DefaultServerAddr), "Webhook server address")

	flag.Parse()

	config = Configuration{
		Repo:                *repo,
		Path:                *path,
		Branch:              *branch,
		CABuntleFile:        *bundleFile,
		Username:            *username,
		Password:            *password,
		PasswordFile:        *passwordFile,
		SSHPrivateKeyFile:   *sshFile,
		InsecureSkipTLS:     *insecure,
		KnownHostsFile:      *knownHosts,
		PollInterval:        *interval,
		WebhookUsername:     *webUsername,
		WebhookPassword:     *webPassword,
		WebhookPasswordFile: *webPasswordFile,
		ServerAddr:          *serverAddr,
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
