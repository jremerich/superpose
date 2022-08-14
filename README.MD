
**⚠️ WARNING: Superpose is currently still in an experimental phase and under heavy development.**
**⚠️ WARNING 2: I'm not a Golang Developer, just getting some fun in a different tech**

# Superpose sync
In quantum superposition a molecule can be in two (or more) quantun states before measurement. I wanna do this with your files :D

## How do I work?
I keep recursively watching all directories that you specify on `watchers.yml` using `inotify` on linux. (Yes, I only work on Linux systems). Look:
```yaml
# ...
watchers:
    - dir: /home/[your-user]/some-dir/
    - dir: ~/.ssh # you can do like this, too!
    - dir: ~/another-dir
      recursive: false # you avoid to watch a dir recursively if you wanna sync only first level.
ignore: # you can avoid syncing some dirs, too.
  - dir: ~/.kube/cache/
# ...
```
When a file is created, updated or deleted on your filesystem, I receive an inotify event and send to a specific folder on your Google Drive.
To configure your Google Drive's info just edit `watchers.yml` like this:
```yaml
# ...
google_drive:
    root_folder_id: [ID of your folder on Google Drive where files will be saved]
    client_id: [your client google client ID]
    client_secret: [your client google client secret]
# ...
```
Once authenticated, `google_drive` will receive all oauth2 data, look:
```yaml
# ...
google_drive:
    root_folder_id: [ID of your folder on Google Drive where files will be saved]
    client_id: [your client google client ID]
    client_secret: [your client google client secret]
    token:
        access_token: [google oauth2 access token]
        token_type: Bearer
        refresh_token: [google oauth2 refresh token]
        expiry: [google oauth2 expiry]
        expires_in: [seconds from now until expiry data]
        scope: https://www.googleapis.com/auth/drive https://www.googleapis.com/auth/drive.activity # minimal needed scopes
# ...
```
Now you has your mapped directories backed up automagically on your Google Drive. If you put me in more workstations with same configurations I'll sync them all using Google Drive Activity API.

I'll keep an SQLite database as cache of informations from Google Drive to avoid API calls since it has limits and improve performance. If you wanna chose where I'll save the `.db` file just add something like this on `watchers.yml`:

```yaml
# ...
config_path: [fullpath of config location]
db: $CONFIG_PATH/[filename to your DB].db
# ...

##Roadmap
 * [x] Load watchers.yml from file
 * [x] Parse paths like '~/'
 * [x] Ignore files and dirs
 * [x] Load ignores from file
 * [x] Get stat info
 * [x] Google drive authentication
 * [x] Send files to remote
 * [x] Save IDs in SQLite
 * [x] Update existing files
 * [x] Delete files on remote
 * [x] Inject metadata on appProperties
 * [x] Refresh token
 * [ ] Check changes on remote using Google Drive Activity API
 * [ ] Download changed files
 * [ ] Read and apply metadata from appProperties
 * [ ] Remove local files when removed or trashed on remote
 * [ ] Do initial download (local folder need be empty)
 * [ ] Save Auth in SQLite and synchronize it
 * [ ] Sync active-active
 * [ ] Do a sync in place of initial download
