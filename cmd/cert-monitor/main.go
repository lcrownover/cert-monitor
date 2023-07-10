package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/exp/slog"

	gomail "gopkg.in/mail.v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	SMTP      SMTPConfig `yaml:"smtp,omitempty"`
	Domains   []string   `yaml:"domains,omitempty"`
	Threshold int        `yaml:"threshold,omitempty"`
}

type SMTPConfig struct {
	Server string   `yaml:"server,omitempty"`
	Port   int      `yaml:"port,omitempty"`
	To     []string `yaml:"to,omitempty"`
	From   string   `yaml:"from,omitempty"`
}

type Domain struct {
	CommonName     string
	DNSNames       []string
	Expires        string
	IsExpiringSoon bool
	Summary        string
}

type configKey struct{}

func main() {
	var (
		config Config
		err    error
		ctx    = context.Background()
	)

	var configFlag = flag.String("config", "", "path to config file")
	var summaryFlag = flag.Bool("summary", false, "show summary information")
	var debugFlag = flag.Bool("debug", false, "show debug output")
	var jsonFlag = flag.Bool("json", false, "format output in json")
	var printFlag = flag.Bool("print", false, "print to stdout instead of email")
	flag.Parse()

	// Configure logging
	var programLevel = new(slog.LevelVar)
	programLevel.Set(slog.LevelWarn)
	var h slog.Handler
	if *jsonFlag {
		h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
	} else {
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
	}
	logger := slog.New(h)
	slog.SetDefault(logger)
	if *debugFlag {
		programLevel.Set(slog.LevelDebug)
	}

	// Load config and store in ctx
	configFilePath := getConfigPath(*configFlag)
	d, err := os.ReadFile(configFilePath)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to read config file: %s\n", configFilePath))
		os.Exit(1)
	}
	err = yaml.Unmarshal(d, &config)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to parse config file: %s\n", err.Error()))
		os.Exit(1)
	}
	ctx = context.WithValue(ctx, configKey{}, &config)

	// check each domain and email if necessary
	var domains []Domain
	for _, cfgDomain := range config.Domains {
		slog.Debug(fmt.Sprintf("checking domain: %s", cfgDomain))

		domain, err := getDomain(ctx, cfgDomain)
		if err != nil {
			slog.Error("failed to dial domain", "error", err.Error())
			continue
		}
		slog.Debug("domain", "domain", domain)

		domains = append(domains, *domain)

		if domain.IsExpiringSoon && !*printFlag {
			subject := fmt.Sprintf("certificate expiration warning: %s", cfgDomain)
			sendEmail(ctx, subject, domain.Summary)
		}
	}

	// email the summary if requested
	if *summaryFlag {
		summaryLines := []string{}
		for _, domain := range domains {
			summaryLines = append(summaryLines, domain.Summary)
			summaryLines = append(summaryLines, "")
		}
		summary := fmt.Sprint(strings.Join(summaryLines, "\n"))
		if *printFlag {
			fmt.Println(summary)
		} else {
			subject := "certificate summary"
			sendEmail(ctx, subject, summary)
		}
	}
}

func getDomain(ctx context.Context, domain string) (*Domain, error) {
	config := ctx.Value(configKey{}).(*Config)
	d := &Domain{}
	summary := []string{}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	conn, err := tls.Dial("tcp", domain+":443", tlsConfig)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	cert := conn.ConnectionState().PeerCertificates[0]
	d.CommonName = cert.Subject.CommonName
	d.DNSNames = cert.DNSNames
	d.Expires = cert.NotAfter.Format("2006-01-02")

	// If the cert is within configured days of expiry
	if isDateWithinDays(ctx, d.Expires, config.Threshold) {
		d.IsExpiringSoon = true
	}

	// build summary
	summary = append(summary, d.CommonName)
	summary = append(summary, fmt.Sprintf("  Expiring Soon: %v", d.IsExpiringSoon))
	summary = append(summary, fmt.Sprintf("  Expires:       %s", d.Expires))
	summary = append(summary, "  DNS Alt Names:")
	for _, dnsName := range d.DNSNames {
		summary = append(summary, fmt.Sprintf("    %s", dnsName))
	}
	d.Summary = strings.Join(summary, "\n")

	return d, nil
}

func sendEmail(ctx context.Context, subject string, contents string) {
	config := ctx.Value(configKey{}).(*Config)
	m := gomail.NewMessage()
	m.SetHeader("From", config.SMTP.From)
    m.SetHeader("To", config.SMTP.To...)
    m.SetHeader("Subject", subject)
    m.SetBody("text/plain", contents)
	slog.Debug("sending email", "subject", subject, "contents", contents)
    d := gomail.NewDialer(config.SMTP.Server, config.SMTP.Port, "", "")
    d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

    if err := d.DialAndSend(m); err != nil {
        slog.Error("failed to send email", "error", err.Error())
    }
}

func isDateWithinDays(ctx context.Context, targetDate string, days int) bool {
	today := time.Now()

	expires, err := time.Parse("2006-01-02", targetDate)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to parse expiration date: %s\n", err.Error()))
	}

	// is the cert within days of expiring?
	return today.AddDate(0, 0, days).After(expires)
}

func getConfigPath(configFlag string) string {
	// We look at 3 places for the config file and use the first defined
	// 1. --config flag
	// 2. CERT_MONITOR_CONFIG_PATH env var
	// 3. /etc/cert-monitor/config.yml

	var defaultConfigFilePath = "/etc/cert-monitor/config.yml"

	if configFlag != "" {
		return configFlag
	}

	configFilePath, found := os.LookupEnv("CERT_MONITOR_CONFIG_PATH")
	if found {
		return configFilePath
	}

	return defaultConfigFilePath
}
