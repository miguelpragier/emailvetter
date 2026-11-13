# Security

Security fixes target the latest released version. Upgrade before reporting a
problem and include the affected version and a minimal reproducer.

Report vulnerabilities privately through GitHub's **Report a vulnerability**
option in this repository's Security tab, when enabled. If that option is
unavailable, open an issue asking the maintainer for a private reporting channel
without disclosing exploit details, credentials, or sensitive data. Do not post
vulnerability details in a public issue or pull request before coordination.

For deployment, set `APP_ENV=production`, configure strong API keys, terminate
HTTPS at a reverse proxy, and restrict database network access. Keep database
authentication and backups under your control. Avoid logging Authorization
headers or full email addresses. A blocklist verdict does not verify mailbox
ownership, existence, or deliverability and should not replace authentication.

Do not submit real secrets in reports. If a credential has been exposed, revoke
or rotate it immediately; deleting a file does not remove it from Git history.
