# Audio

Stash can keep audio such as music, audiobooks and ASMR in a library of its own, next to your scenes and images.

## Adding audio

Audio is picked up by the normal [Scan](/settings?tab=tasks) task. To leave a library out, untick **Audio** for it in the library settings.

These file extensions are scanned by default, and can be changed under **Settings > Library > Audio extensions**:

`mp3`, `mpa`, `m4a`, `m4b`, `aac`, `flac`, `wav`, `aif`, `aiff`, `ogg`, `oga`, `opus`, `wma`, `ape`, `mka`

## Details read from the file

The first time a file is scanned, Stash fills in what it can from the file's tags:

| Tag | Becomes |
|---|---|
| Title | Title |
| Comment | Details |
| Date | Date |
| Artist | Performer |
| Genre | Tag |
| Album | Group |
| Track number | Position in the group |
| Embedded picture | Cover image |

Artists, genres and albums are only linked when a performer, tag or group of that name already exists. A scan doesn't create new ones. Rescanning doesn't overwrite an audio you've already got in your library.

## Playing

Files play directly when your browser can decode them. When it can't, the player switches to an MP3 conversion made on the fly by the server, so a file such as ALAC or WMA still plays and can be skipped through. The conversion uses more of the server's processor than playing the file directly.

Your listening position, play count and rating belong to your own account. Other accounts have their own.

## Playing on other devices

If DLNA is enabled, audio appears in its own **Audio** folder on DLNA players. Plays on a DLNA player aren't added to your play count.
