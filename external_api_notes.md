# Verified external API notes

## Nayan media downloader package
Source package: https://www.npmjs.com/package/nayan-media-downloaders
Source repository referenced by package: https://github.com/MOHAMMAD-NAYAN/nayan-videos-downloader

The package source (`src/index.js`) defines:
- `apiBaseUrl = https://nayan-video-downloader.vercel.app/`
- `ytdown` calls `GET https://nayan-video-downloader.vercel.app/ytdown?url=<YOUTUBE_URL>`.
- The wrapper returns the JSON response directly and reports `{status:false}` on request failure.

## User repository
Source: https://github.com/uf-prince/fast-yt-api
The repository README documents:
- `GET /info?url=YOUTUBE_URL`
- `GET /download?url=YOUTUBE_URL&type=video&quality=360`

## Local tests
- WhiteShadow API returned success for the screenshot video but every returned media URL tested HTTP 403.
- Local fast-yt-api fallback returned complete 360p data for `dQw4w9WgXcQ`, but the screenshot video stream closed early with curl error 18.
- The known old Railway fallback URL returned HTTP 404 (application not found).
