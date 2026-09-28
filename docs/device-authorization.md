# Browser-approved device authorization

The Hub supports the OAuth 2.0 device authorization order: an HTTP client asks for a code, the human signs in at `/activate` and approves the displayed permissions, and the same client polls `/oauth/token`. The Hub issues its own scoped credentials. GitHub is used only to confirm that the browser user is the configured owner; GitHub tokens are discarded after the identity check.

This feature is disabled until all four settings are present:

| Setting | Value |
| --- | --- |
| `HUB_PUBLIC_URL` | Public origin, `https://agent.gwinam.com` in production |
| `HUB_GITHUB_CLIENT_ID` | GitHub OAuth app client ID |
| `HUB_GITHUB_CLIENT_SECRET` | GitHub OAuth app secret, supplied from a secret store |
| `HUB_GITHUB_OWNER_ID` | Numeric GitHub user ID permitted to approve devices |

Register a GitHub OAuth app with callback URL `https://agent.gwinam.com/auth/github/callback`. Configure the GitHub user ID, not the mutable login name. Production rejects an HTTP public URL. The approval screen currently grants access to the bootstrapped owner's `personal` space. Browser sessions last 30 minutes and are invalidated when the Hub restarts.

The production deploy script reads `/agent-control-plane/github_client_id`, `/agent-control-plane/github_client_secret`, and `/agent-control-plane/github_owner_id` as SSM parameters. Add the client secret as a SecureString and create `github_client_id` last to enable the feature; an absent client ID leaves existing bearer-token access running. The script fails if an enabled configuration is incomplete. No GitHub OAuth credentials are committed to the repository.

## Client exchange

1. `POST /oauth/device_authorization` as `application/x-www-form-urlencoded` with `client_id=agent-control-plane`, a `client_label`, and a space-separated `scope` containing `context:read` and/or `work:write`. The response contains `device_code`, `user_code`, `verification_uri`, `expires_in=600`, and `interval=5`.
2. Show only `verification_uri` and `user_code` to the human. Keep `device_code` inside the credential-handling integration. The human visits `/activate`, signs in through GitHub, enters the code, checks the client label and requested permissions, and approves or denies.
3. Poll `POST /oauth/token` with `client_id=agent-control-plane`, `grant_type=urn:ietf:params:oauth:grant-type:device_code`, and the `device_code`. Respect the returned interval and `slow_down`. Approval returns a 15-minute bearer access token and a rotating refresh token. The device code can be redeemed only once.
4. Before access expiry, call the same endpoint with `grant_type=refresh_token` and the current `refresh_token`. Replace both stored values atomically. Refresh credentials expire after 90 days of inactivity. Reusing an old refresh token revokes the device grant.

The credential integration must store `device_code`, `access_token`, and `refresh_token` outside prompts, agent transcripts, repositories, shell history, and logs. The short `user_code` is the only code intended for display. An agent with raw HTTP tools but no protected credential store can read the OpenAPI contract, but cannot safely persist automatic login. An optional local credential bridge or a supported agent's native OAuth integration can handle that boundary without changing the Hub API.

For macOS or Linux agents without native credential handling, the optional Python bridge needs no compilation. It stores credentials in `~/.config/agent-control-plane/credentials.json` with owner-only file permissions and serializes refreshes across local processes. It prints only the verification address and user code; API calls print only work API responses. A machine with a native credential store can replace this bridge without changing the server. The bridge is intended for a trusted local user account and full-disk-encrypted computer; it is not an OS keychain integration.

```sh
python3 scripts/hub-credentials.py start --label "Claude on laptop" --scope "context:read work:write"
# Open the displayed address, sign in, enter the code, and approve.
python3 scripts/hub-credentials.py finish
python3 scripts/hub-credentials.py api GET '/api/v1/tasks?status=active&limit=20'
```

The browser owner can see and revoke connected installations at `/devices`. The existing `hub admin revoke-token` command also revokes a device token. Revocation stops both API access and refresh. Each approved installation retains one client ID while its access token rotates, so work sessions remain owned by the same installation.

The first owner still needs operator bootstrap. The device endpoints do not permit anonymous registration: a device only receives credentials after the configured GitHub owner explicitly approves it. The Hub does not run agents or make work decisions.
