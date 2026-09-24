# Production deployment on the existing EC2 instance

The target is the `prod` account's existing ARM64 `t4g.small` managed by `HyeonTee/aws/prod/justice`. Justice keeps its own Compose project, PostgreSQL, credentials, port 80, and backups. The Hub uses `agent-control-plane-prod`, a separate PostgreSQL data directory, and port 443. PostgreSQL has no host port. An additional security group admits port 443 only from the CloudFront origin-facing prefix list; the Justice group stays on port 80. They must be separate groups because each CloudFront prefix-list rule has [weight 55 against the usual 60-rule security-group quota](https://docs.aws.amazon.com/vpc/latest/userguide/working-with-aws-managed-prefix-lists.html).

The public path is `agent.gwinam.com → CloudFront + per-IP WAF rate rule → HTTPS origin.agent.gwinam.com:443 → Nginx → Hub`. CloudFront disables caching for every path, forwards bearer authorization, and overwrites the `X-Origin-Secret` header. The Hub rejects requests without the matching secret; the security group alone cannot distinguish another CloudFront distribution. The origin uses an exportable ACM public certificate, stored only on the instance and refreshed daily from ACM. [CloudFront requires a trusted, matching certificate for HTTPS custom origins](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/using-https-cloudfront-to-custom-origin.html). Exportable ACM certificates have an [issuance and renewal charge](https://aws.amazon.com/certificate-manager/pricing/), and [AWS WAF adds Web ACL, rule, and request charges](https://aws.amazon.com/waf/pricing/).

The Hub is a passive data service. Deployment and backup scripts operate its own containers and database; the HTTP API never executes an agent or project command.

## Deployment order

1. Sign in to AWS SSO for profile `personal`. Inspect the live instance's free RAM and disk, existing listeners, Docker container sizes, and Justice health through SSM. Do not apply the new stack if a second PostgreSQL instance and restore-check database will not fit.
2. Review and apply `HyeonTee/aws/prod/justice` to attach the Hub-only security group to the existing instance. This must be an in-place change; stop if the plan replaces the instance, changes its EIP, or alters Justice's port-80 rule.
3. Review and apply `HyeonTee/aws/prod/agent-control-plane`. It owns the CloudFront distribution, both certificates, DNS records, rate rule, ECR repository, backup bucket, Hub parameters, and narrowly scoped deploy/instance policies. Keep secrets out of `tofu output` and logs.
4. Set the four GitHub repository variables from that stack's `github_variables` output: `AWS_DEPLOY_ROLE_ARN`, `ECR_REPOSITORY_URL`, `EC2_INSTANCE_ID`, and `DEPLOY_BUCKET`. Trigger the manual `deploy` workflow from the reviewed `main` commit. It builds an ARM64 image, uploads the deployment bundle to S3, and runs the instance deployment through SSM. It does not touch Justice's Compose project.
5. The deployment exports the origin certificate, checks HTTPS readiness, takes a database backup to S3, restores it into a disposable database, and enables three systemd timers. Only after all checks pass should an operator bootstrap an owner and issue a scoped client token. Verify public HTTPS reads and writes with a separate integration token, then revoke the test token.

The workflow is manual until the first deployment and cross-computer client check pass. An SSM failure or failed restore must leave the deployment marked failed; do not expose a token until the cause is fixed.

## Backup and recovery

`agent-hub-backup.timer` writes a PostgreSQL custom-format dump to the private S3 bucket nightly. S3 server-side encryption and a 30-day lifecycle policy apply. `agent-hub-restore-check.timer` downloads the latest archive weekly, restores it into `hub_restore_check`, verifies migrations and work tables can be queried, and removes that disposable database. The first deploy runs both checks immediately. The instance role can read and write only this Hub's backup prefix and read its deployment bundle.

For an actual recovery, stop the Hub writer, preserve the damaged database for investigation, create a fresh Hub database on a new or repaired instance, download a known-good `backups/hub-*.dump`, and run `pg_restore --exit-on-error` into it. Verify migrations, task and checkpoint counts, and authentication with a rotated token before routing CloudFront to the recovered origin. The weekly disposable restore checks archive usability; it does not test a full EC2 rebuild or DNS failover. That rebuild remains a separate operational drill.

## Current state

The production files are prepared but have not been applied to AWS. Local development continues to use `compose.yaml` bound to `127.0.0.1` and its own password.
