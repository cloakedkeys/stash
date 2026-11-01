# Video upload

Administrators can upload videos from **Scenes → New** (`/scenes/new`). Choose a library that allows video, select one or more files, then choose **Upload videos**. Files upload one at a time. Each file shows a pending state and its result; successful scans include links to their scenes.

Uploads go directly into the selected configured library root. Subfolders are not accepted. Filenames must be valid on Windows as well as your server's operating system, and must have a configured video extension. Existing names are refused, including when two uploads arrive at once; files are never overwritten or renamed automatically.

The page shows the server's current upload request limit. It defaults to **1024 MiB (1 GiB)**, including multipart form overhead, so the file must be slightly smaller. To raise it, set `max_upload_size` in `config.yml` to the desired number of MiB (for example, `max_upload_size: 4096` for 4 GiB), then restart Stash. A reverse proxy may impose a smaller limit of its own.

After saving, Stash scans only the uploaded file. It does not run a full library scan or auto-tag. If scanning fails, the result says that the file was saved and gives its path. The saved file remains there; uploading the same name again will fail. Resolve the scan error and scan that file through the existing scan tools.

Stash writes a temporary file in the destination directory, publishes it without replacing an existing name, and removes the temporary name. A failed save removes the temporary file. Larger uploads are first held in the system's temporary folder, so both that folder (inside the container when using Docker) and the library need enough free space for the file.

This upload supports video only. It does not upload images, audio or archives, fetch URLs, resume transfers, create subfolders, or permit non-admin accounts to upload.
