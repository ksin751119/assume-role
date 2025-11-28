This tool will request and set temporary credentials in your shell environment variables for a given role.

## Requirements

- Go 1.25+
- AWS credentials configured in `~/.aws/credentials`

## Installation

### Using Go Install (Recommended)

```bash
go install github.com/ksin751119/assume-role@latest
```

### Building from Source

```bash
git clone https://github.com/ksin751119/assume-role.git
cd assume-role
go build -o bin/assume-role .
```

## Configuration

Setup a profile for each role you would like to assume in `~/.aws/config`.

**Important:** For profiles that use `role_arn` (assume role), you **must** use the `[profile <name>]` format. This is required by AWS SDK v2.

For example:

`~/.aws/config`:

```ini
[default]
region = {your_region}

[profile stage]
# Stage AWS Account - uses [profile ...] format because it has role_arn
region = {your_region}
role_arn = arn:aws:iam::{stage_account_id}:role/{role_name}
source_profile = default

[profile prod]
# Production AWS Account - with MFA
region = {your_region}
role_arn = arn:aws:iam::{prod_account_id}:role/{role_name}
mfa_serial = arn:aws:iam::{mfa_account_id}:mfa/{your_username}
source_profile = default
```

`~/.aws/credentials`:

```ini
[default]
aws_access_key_id = {your_access_key_id}
aws_secret_access_key = {your_secret_access_key}
```

> **Note:** The `[default]` profile does not need the `profile` prefix, but all other profiles with `role_arn` must use `[profile <name>]` format.

Reference: https://docs.aws.amazon.com/cli/latest/userguide/cli-roles.html

In this example, we have three AWS Account profiles:

 * default - base credentials
 * stage - assumes SuperUser role in stage account
 * prod - assumes SuperUser role in prod account (with MFA)

Each member of the org has their own IAM user and access/secret key stored in `~/.aws/credentials`.

The `stage` and `prod` AWS Accounts have an IAM role named `SuperUser`.
The `assume-role` tool helps a user authenticate (using their keys) and then assume the privilege of the `SuperUser` role, even across AWS accounts!

## Usage

```
assume-role [options] <role> [<command> <args...>]
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `-duration` | `1h` | The duration that the credentials will be valid for (e.g., `30m`, `2h`) |
| `-format` | `bash` | Output format: `bash`, `fish`, or `powershell` |

### Examples

Perform an action as the given IAM role:

```bash
$ assume-role stage aws iam get-user
```

The `assume-role` tool sets `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and `AWS_SESSION_TOKEN` environment variables and then executes the command provided.

If the role requires MFA, you will be asked for the token first:

```bash
$ assume-role prod aws iam get-user
MFA code: 123456
```

If no command is provided, `assume-role` will output the temporary security credentials:

```bash
$ assume-role prod
export AWS_ACCESS_KEY_ID="ASIAI....UOCA"
export AWS_SECRET_ACCESS_KEY="DuH...G1d"
export AWS_SESSION_TOKEN="AQ...1BQ=="
export AWS_SECURITY_TOKEN="AQ...1BQ=="
export ASSUMED_ROLE="prod"
# Run this to configure your shell:
# eval $(assume-role prod)
```

Or windows PowerShell:
```cmd
$env:AWS_ACCESS_KEY_ID="ASIAI....UOCA"
$env:AWS_SECRET_ACCESS_KEY="DuH...G1d"
$env:AWS_SESSION_TOKEN="AQ...1BQ=="
$env:AWS_SECURITY_TOKEN="AQ...1BQ=="
$env:ASSUMED_ROLE="prod"
# Run this to configure your shell:
# assume-role.exe prod | Invoke-Expression
```

### Using with custom duration

Request credentials valid for 2 hours:

```bash
$ assume-role -duration 2h prod aws s3 ls
```

### Using with different shell formats

For fish shell:
```bash
$ assume-role -format fish prod
set -gx AWS_ACCESS_KEY_ID "ASIAI....UOCA";
set -gx AWS_SECRET_ACCESS_KEY "DuH...G1d";
...
```

### Using with Role ARN directly

You can also specify a role ARN directly instead of a profile name:

```bash
$ assume-role arn:aws:iam::123456789012:role/MyRole aws sts get-caller-identity
```

### Shell Aliases

If you use `eval $(assume-role)` frequently, you may want to create an alias for it:

* zsh
```shell
alias assume-role='function(){eval $(command assume-role $@);}'
```
* bash
```shell
function assume-role { eval $( $(which assume-role) $@); }
```
* fish
```shell
function assume-role
    eval (command assume-role -format fish $argv)
end
```

## Development

### Build

```bash
# Build for current platform
go build -o bin/assume-role .

# Build for all platforms (Linux, macOS, Windows)
make bin

# Run tests
make test
```

### Project Structure

This project uses:
- **Go 1.25** with Go Modules
- **AWS SDK for Go v2** for AWS API interactions
- **gopkg.in/yaml.v3** for YAML parsing

## TODO

* [ ] Cache credentials.
