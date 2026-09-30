# gitlab-to-gitea

Go-based tool for migrating GitLab repositories, users, groups, issues and related data to Gitea instances.

More-or-less a port of [gitlab-to-gitea](https://git.autonomic.zone/kawaiipunk/gitlab-to-gitea) from python to Go because *fixing* python appears to be a thing I just can't get my mind around, but *rewriting* it? I'm actually OK at that.

Also includes:
 - `cmd/forkfix`, for fixing fork relationships between migrated repositories by manipulating the gitea sql database
 - `cmd/unmigrate` to delete everything from a gitea instance except for the admin users
 - `cmd/mirror` to set up a mirror from github to gitea
 - `cmd/orgfix` to establish an organization's repositories as the "parent fork" by manipulating the gitea sql database
 - `cmd/namefix` to discover repositories that have identical initial commit hashes but different names
 - `cmd/johnconnor` which is a super-dangerous script for eliminating spam accounts from gitlab instances.

## Core Functionality

- Migrates users, groups, and their relationships from GitLab to Gitea
- Transfers repositories via Gitea's native GitLab migration service, bringing over issues, merge requests, labels, milestones, releases, wiki and LFS objects
- Preserves user relationships (collaborators) and SSH keys
- Supports resumable migrations through state tracking
- Handles username normalization and entity mapping between platforms

## Go Implementation Improvements

- Modular package structure instead of monolithic script
- Configuration via environment variables rather than hardcoded values
- Added utility tools (`forkfix`, `unmigrate`, `johnconnor`, `mirror`, `orgfix`, `namefix`) 
- Database connectivity for commit action imports
- Improved error handling with recovery mechanisms
- Separation of API client code from migration logic

## Installation

1. Ensure Go 1.24+ is installed
2. Clone the repository:
   ```bash
   git clone https://github.com/go-i2p/gitlab-to-gitea.git
   cd gitlab-to-gitea
   ```
3. Install dependencies:
   ```bash
   go mod download
   ```
4. Build the executable:
   ```bash
   go build -o gitlab-to-gitea ./cmd/migrate/
   ```

## Configuration

1. Copy the example environment file:
   ```bash
   cp _env.example .env
   ```
2. Edit `.env` with your GitLab and Gitea details:
   ```
   GITLAB_URL=https://your-gitlab-instance.com
   GITLAB_TOKEN=your-gitlab-token
   GITEA_URL=https://your-gitea-instance.com
   GITEA_TOKEN=your-gitea-token
   ```
   `GITLAB_TOKEN` is also passed to Gitea's importer, so it needs the `read_api` and `read_repository` scopes and access to every project being migrated. The Gitea instance must be able to reach `GITLAB_URL` (allow it via `[migrations] ALLOWED_DOMAINS` / `ALLOW_LOCALNETWORKS` if needed). Large projects can take a while; tune `MIGRATION_TIMEOUT` (default `2h`).

### Self-signed or otherwise invalid TLS certificates

TLS certificates are verified by default. To skip verification:

- For this tool's own API calls to GitLab and Gitea, set `INSECURE_SKIP_TLS_VERIFY=true` or pass `-insecure-skip-tls-verify` to the migrate command.
- For the repository import itself, Gitea connects to GitLab directly, so this tool's setting does not apply. Set it in Gitea's `app.ini` and restart Gitea:
  ```ini
  [migrations]
  SKIP_TLS_VERIFY = true
  ```

Only use these against servers you trust. They make the connections open to man-in-the-middle attacks.

## Usage

Execute the migration tool after configuration:

```bash
./gitlab-to-gitea
```

The tool will:
1. Connect to both GitLab and Gitea instances
2. Migrate users and groups first
3. Migrate each project with Gitea's native GitLab importer (`POST /repos/migrate` with `service: "gitlab"`), then map project members to collaborators
4. Track progress in `migration_state.json` (resumable if interrupted)

## Key Dependencies

- github.com/xanzy/go-gitlab: GitLab API client
- github.com/joho/godotenv: Environment variable handling
- github.com/go-sql-driver/mysql: Optional database connectivity for action import
- github.com/mattn/go-sqlite3: forkfix sqlite handling

## Optional Features

For commit action import to Gitea's activity timeline:
1. Configure database details in `.env`
2. Generate a commit log file
3. Use the database import functionality in the `gitea` package

## License

MIT License