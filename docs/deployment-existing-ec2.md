# Initial deployment target: existing EC2 instance

The first deployment is planned for the `prod` account's existing `t4g.small` instance managed by `HyeonTee/aws/prod/justice`. This is an ARM64 machine with 2 GiB of memory and a 20 GiB encrypted root volume. Justice already runs its API, PostgreSQL, and backup containers there. Its Compose stack binds host port 80; its security group accepts that port only from the CloudFront origin-facing prefix list. Instance access and deployments use SSM, not SSH.

Agent Control Plane must have its own Compose project, PostgreSQL data volume, database credentials, backup, ECR repository, and deployment role. It must not join Justice's database or allow one deployment's `docker compose --remove-orphans` to affect the other. Keep the Hub's PostgreSQL port on the private Compose network. Reserve a different host port for the Hub origin and check available memory and disk space on the instance before applying infrastructure.

The proposed public route is a separate CloudFront distribution and certificate for `agent.gwinam.com`, with a Route 53 alias record owned by a new `prod/agent-control-plane` OpenTofu stack. CloudFront supports a custom origin port, so the existing Justice listener can remain on port 80. Restrict the new origin port to CloudFront origin-facing addresses and require a distribution-specific origin secret at the Hub or a front proxy. A CloudFront prefix list alone permits traffic from other CloudFront distributions.

The origin transport still needs a final choice before exposing project data. CloudFront-to-EC2 HTTPS requires a valid certificate on the origin and an automated renewal path; plain HTTP would leave that hop unencrypted. The current Phase 0 server is local-only and does not expose project data. Do not apply a public distribution for it until origin transport, token handling, backup, and a capacity check have been completed.

The local `compose.yaml` is for development and binds only to `127.0.0.1`. Production deployment configuration and infrastructure will be added after the authenticated work-continuity API is ready for a security review.
