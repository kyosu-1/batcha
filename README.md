# batcha

Declarative deployment tool for AWS Batch Job Definitions.

Inspired by [ecspresso](https://github.com/kayac/ecspresso). Manage your AWS Batch Job Definitions as code with template rendering and Terraform state integration.

## Install

### Homebrew (macOS / Linux)

```
brew install kyosu-1/tap/batcha
```

### Go

```
go install github.com/kyosu-1/batcha/cmd/batcha@latest
```

### Binary

Download from [Releases](https://github.com/kyosu-1/batcha/releases).

## Quick Start

### From an existing AWS Batch Job Definition

```
batcha init --job-definition-name my-job-def
```

This generates `batcha.yml` and `job-definition.json` from the active definition on AWS.

### From scratch

1. Create a config file (`batcha.yml`):

```yaml
region: ap-northeast-1
job_definition: job-definition.json
```

2. Create a job definition template (`job-definition.json`):

```json
{
  "jobDefinitionName": "{{ must_env `JOB_NAME` }}",
  "type": "container",
  "containerProperties": {
    "image": "{{ must_env `IMAGE` }}",
    "resourceRequirements": [
      { "type": "VCPU", "value": "1" },
      { "type": "MEMORY", "value": "2048" }
    ],
    "environment": [
      { "name": "APP_ENV", "value": "{{ env `APP_ENV` `production` }}" }
    ]
  }
}
```

### Deploy

```
batcha diff --config batcha.yml      # Preview changes
batcha register --config batcha.yml  # Register (skips if no changes)
```

## Commands

| Command | Description |
|---|---|
| `batcha init --job-definition-name <name>` | Generate config and template from an existing AWS Batch definition |
| `batcha register --config <file>` | Register a Job Definition to AWS Batch (skips if no changes) |
| `batcha register --config <file> --dry-run` | Preview the rendered JSON without registering |
| `batcha render --config <file>` | Render and print the job definition template |
| `batcha diff --config <file>` | Show diff between local template and active AWS definition |
| `batcha version` | Print version |

## Configuration

### Config file

```yaml
region: ap-northeast-1          # AWS region (falls back to AWS_REGION env var)
job_definition: job-def.json    # Path to job definition template (relative to config file)
plugins:
  - name: tfstate
    config:
      url: s3://my-bucket/terraform.tfstate
```

### Template functions

batcha uses [kayac/go-config](https://github.com/kayac/go-config) for template rendering. Available functions:

| Function | Description |
|---|---|
| `env KEY DEFAULT` | Read environment variable with optional default |
| `must_env KEY` | Read environment variable (fails if not set) |

### Terraform state integration

With the `tfstate` plugin, you can reference Terraform state values:

```json
{
  "containerProperties": {
    "executionRoleArn": "{{ tfstate `aws_iam_role.batch_exec.arn` }}"
  }
}
```

Supports S3, local, GCS, AzureRM, and Terraform Cloud backends via [fujiwara/tfstate-lookup](https://github.com/fujiwara/tfstate-lookup).

### Key conversion

batcha automatically converts camelCase keys in your JSON template to PascalCase for AWS SDK v2 compatibility. Write your templates in camelCase:

```json
{ "jobDefinitionName": "..." }
```

This will be sent to the API as `{ "JobDefinitionName": "..." }`.

Keys under `tags`, `parameters`, and `options` are preserved as-is.

## GitHub Actions

```yaml
- uses: kyosu-1/batcha@v0
  with:
    version: v0.1.0
```

| Input | Description | Default |
|---|---|---|
| `version` | Version to install | `latest` |
| `version-file` | File containing the version (e.g. `.batcha-version`) | |
| `args` | Arguments to pass to batcha | |

### Examples

Install and run:

```yaml
- uses: kyosu-1/batcha@v0
  with:
    args: "register --config batcha.yml --dry-run"
```

Install only, then run separately:

```yaml
- uses: kyosu-1/batcha@v0
- run: batcha register --config batcha.yml
```

## License

MIT
