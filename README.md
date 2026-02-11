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
| `batcha status --config <file>` | Show current status of the job definition on AWS |
| `batcha run --config <file> [--job-queue <queue>]` | Submit a job using the latest active job definition |
| `batcha logs --config <file> [--job-id <id>]` | Fetch CloudWatch logs for a Batch job |
| `batcha verify --config <file>` | Validate the job definition template locally (no AWS calls) |
| `batcha version` | Print version |

### run

Submit a job to AWS Batch using the latest active revision of the job definition.

```
batcha run --config batcha.yml --job-queue my-queue
```

| Flag | Description | Required |
|---|---|---|
| `--config` | Path to config YAML file | Yes |
| `--job-queue` | AWS Batch job queue name (overrides config) | No* |
| `--job-name` | Job name (defaults to job definition name) | No |
| `--parameter` | Parameter overrides as `key=value` (repeatable) | No |
| `--wait` | Wait for the job to complete and report status | No |

*`--job-queue` is required unless `job_queue` is set in config.

With `--wait`, batcha polls the job status every 10 seconds and exits with code 0 on success or 1 on failure.

```
batcha run --config batcha.yml --job-queue my-queue --wait --parameter input=s3://bucket/file.csv
```

### logs

Fetch CloudWatch logs for a Batch job.

```
batcha logs --config batcha.yml --job-id <job-id>
```

| Flag | Description | Required |
|---|---|---|
| `--config` | Path to config YAML file | Yes |
| `--job-id` | AWS Batch job ID (if omitted, finds the latest job) | No |
| `--job-queue` | AWS Batch job queue name (overrides config, used for latest job search) | No |
| `-f`, `--follow` | Follow logs in real time | No |
| `--since` | Show logs since duration (e.g. `1h`, `30m`) | No |

Without `--job-id`, batcha searches for the most recent job matching the configured job definition in the specified queue.

```
batcha logs --config batcha.yml --follow
batcha logs --config batcha.yml --since 30m
```

### verify

Validate the job definition template locally without calling AWS. Useful in CI pipelines.

```
batcha verify --config batcha.yml
```

Checks:

- Template rendering (syntax errors, missing `must_env` variables)
- Valid `RegisterJobDefinitionInput` structure
- Required fields (`jobDefinitionName`, `type`, `containerProperties.image`, etc.)
- Resource requirements (`VCPU` and `MEMORY` present and valid)
- Fargate constraints (valid vCPU/memory combinations, `executionRoleArn` required)

## Configuration

### Config file

```yaml
region: ap-northeast-1          # AWS region (falls back to AWS_REGION env var)
job_definition: job-def.json    # Path to job definition template (relative to config file)
job_queue: my-job-queue         # Default job queue for run/logs commands (optional)
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
    version: v0.3.0
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
