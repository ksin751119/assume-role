package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"gopkg.in/yaml.v3"
)

var (
	configFilePath = fmt.Sprintf("%s/.aws/roles", os.Getenv("HOME"))
	roleArnRe      = regexp.MustCompile(`^arn:aws:iam::(.+):role/([^/]+)(/.+)?$`)
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <role> [<command> <args...>]\n", os.Args[0])
	flag.PrintDefaults()
}

func init() {
	flag.Usage = usage
}

func defaultFormat() string {
	var shell = os.Getenv("SHELL")

	switch runtime.GOOS {
	case "windows":
		if os.Getenv("SHELL") == "" {
			return "powershell"
		}
		fallthrough
	default:
		if strings.HasSuffix(shell, "fish") {
			return "fish"
		}
		return "bash"
	}
}

func main() {
	var (
		duration = flag.Duration("duration", time.Hour, "The duration that the credentials will be valid for.")
		format   = flag.String("format", defaultFormat(), "Format can be 'bash' or 'powershell'.")
	)
	flag.Parse()
	argv := flag.Args()
	if len(argv) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	role := argv[0]
	args := argv[1:]

	// Load credentials from configFilePath if it exists, else use regular AWS config
	var creds *aws.Credentials
	var err error
	if roleArnRe.MatchString(role) {
		creds, err = assumeRole(ctx, role, "", *duration)
	} else if _, err = os.Stat(configFilePath); err == nil {
		fmt.Fprintf(os.Stderr, "WARNING: using deprecated role file (%s), switch to config file"+
			" (https://docs.aws.amazon.com/cli/latest/userguide/cli-roles.html)\n",
			configFilePath)
		var cfg roleConfigMap
		cfg, err = loadConfig()
		must(err)

		roleCfg, ok := cfg[role]
		if !ok {
			must(fmt.Errorf("%s not in %s", role, configFilePath))
		}

		creds, err = assumeRole(ctx, roleCfg.Role, roleCfg.MFA, *duration)
	} else {
		creds, err = assumeProfile(ctx, role, *duration)
	}

	must(err)

	if len(args) == 0 {
		switch *format {
		case "powershell":
			printPowerShellCredentials(role, creds)
		case "bash":
			printCredentials(role, creds)
		case "fish":
			printFishCredentials(role, creds)
		default:
			flag.Usage()
			os.Exit(1)
		}
		return
	}

	err = execWithCredentials(role, args, creds)
	must(err)
}

func execWithCredentials(role string, argv []string, creds *aws.Credentials) error {
	argv0, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}

	os.Setenv("AWS_ACCESS_KEY_ID", creds.AccessKeyID)
	os.Setenv("AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey)
	os.Setenv("AWS_SESSION_TOKEN", creds.SessionToken)
	os.Setenv("AWS_SECURITY_TOKEN", creds.SessionToken)
	os.Setenv("ASSUMED_ROLE", role)

	env := os.Environ()
	return syscall.Exec(argv0, argv, env)
}

// printCredentials prints the credentials in a way that can easily be sourced
// with bash.
func printCredentials(role string, creds *aws.Credentials) {
	fmt.Printf("export AWS_ACCESS_KEY_ID=\"%s\"\n", creds.AccessKeyID)
	fmt.Printf("export AWS_SECRET_ACCESS_KEY=\"%s\"\n", creds.SecretAccessKey)
	fmt.Printf("export AWS_SESSION_TOKEN=\"%s\"\n", creds.SessionToken)
	fmt.Printf("export AWS_SECURITY_TOKEN=\"%s\"\n", creds.SessionToken)
	fmt.Printf("export ASSUMED_ROLE=\"%s\"\n", role)
	fmt.Printf("# Run this to configure your shell:\n")
	fmt.Printf("# eval $(%s)\n", strings.Join(os.Args, " "))
}

// printFishCredentials prints the credentials in a way that can easily be sourced
// with fish.
func printFishCredentials(role string, creds *aws.Credentials) {
	fmt.Printf("set -gx AWS_ACCESS_KEY_ID \"%s\";\n", creds.AccessKeyID)
	fmt.Printf("set -gx AWS_SECRET_ACCESS_KEY \"%s\";\n", creds.SecretAccessKey)
	fmt.Printf("set -gx AWS_SESSION_TOKEN \"%s\";\n", creds.SessionToken)
	fmt.Printf("set -gx AWS_SECURITY_TOKEN \"%s\";\n", creds.SessionToken)
	fmt.Printf("set -gx ASSUMED_ROLE \"%s\";\n", role)
	fmt.Printf("# Run this to configure your shell:\n")
	fmt.Printf("# eval (%s)\n", strings.Join(os.Args, " "))
}

// printPowerShellCredentials prints the credentials in a way that can easily be sourced
// with Windows powershell using Invoke-Expression.
func printPowerShellCredentials(role string, creds *aws.Credentials) {
	fmt.Printf("$env:AWS_ACCESS_KEY_ID=\"%s\"\n", creds.AccessKeyID)
	fmt.Printf("$env:AWS_SECRET_ACCESS_KEY=\"%s\"\n", creds.SecretAccessKey)
	fmt.Printf("$env:AWS_SESSION_TOKEN=\"%s\"\n", creds.SessionToken)
	fmt.Printf("$env:AWS_SECURITY_TOKEN=\"%s\"\n", creds.SessionToken)
	fmt.Printf("$env:ASSUMED_ROLE=\"%s\"\n", role)
	fmt.Printf("# Run this to configure your shell:\n")
	fmt.Printf("# %s | Invoke-Expression \n", strings.Join(os.Args, " "))
}

// assumeProfile assumes the named profile which must exist in ~/.aws/config
// (https://docs.aws.amazon.com/cli/latest/userguide/cli-roles.html) and returns the temporary STS
// credentials.
func assumeProfile(ctx context.Context, profile string, duration time.Duration) (*aws.Credentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			o.TokenProvider = func() (string, error) {
				return readTokenCode()
			}
			o.Duration = duration
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config for profile %s: %w", profile, err)
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("retrieving credentials: %w", err)
	}
	return &creds, nil
}

// assumeRole assumes the given role and returns the temporary STS credentials.
func assumeRole(ctx context.Context, role, mfa string, duration time.Duration) (*aws.Credentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := sts.NewFromConfig(cfg)

	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(role),
		RoleSessionName: aws.String("cli"),
		DurationSeconds: aws.Int32(int32(duration / time.Second)),
	}
	if mfa != "" {
		input.SerialNumber = aws.String(mfa)
		token, err := readTokenCode()
		if err != nil {
			return nil, fmt.Errorf("reading MFA token: %w", err)
		}
		input.TokenCode = aws.String(token)
	}

	resp, err := client.AssumeRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("assuming role %s: %w", role, err)
	}

	return &aws.Credentials{
		AccessKeyID:     aws.ToString(resp.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(resp.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(resp.Credentials.SessionToken),
	}, nil
}

type roleConfig struct {
	Role string `yaml:"role"`
	MFA  string `yaml:"mfa"`
}

type roleConfigMap map[string]roleConfig

// readTokenCode reads the MFA token from Stdin.
func readTokenCode() (string, error) {
	r := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "MFA code: ")
	text, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// loadConfig loads the ~/.aws/roles file.
func loadConfig() (roleConfigMap, error) {
	raw, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}

	cfg := make(roleConfigMap)
	return cfg, yaml.Unmarshal(raw, &cfg)
}

func must(err error) {
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Errors are already on Stderr.
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
