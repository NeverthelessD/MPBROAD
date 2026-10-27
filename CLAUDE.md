# MyPhotoBook_ROAD — notes for Claude

**Read `PROJECT_NOTES.md` first** (structure, decisions, progress checklist) and update it at the end of every task.
MyPhotoBook_ROAD moves photos and videos from an iPhone (USB cable) to a Mac (Apple Silicon), sorts them into
year/month/day folders, skips what was already moved, keeps Live Photo pairs together, verifies every copy and only then
offers to delete from the iPhone. It is a personal app for **Nevertheless_D**, who is not a programmer.
Talk to them in Korean, in plain words. Never ask them to use Terminal. Never mention GitHub (or the repo name) in anything the user sees.

It follows the MSROAD 1.1+ pattern (see that repo): **a real, visible Dock app** — Swift shell `app/shell/main.swift`
(WKWebView window) → Go engine supervisor → Go engine, all children of the app, only while the app is open
(macOS 27 "Background App Activity" rule; never add hidden helpers or SIGTERM-ignoring).
iPhone access uses Apple's ImageCaptureCore, which Go can't call, so the engine runs a small Swift helper
`mpbroad-device` (app/device/main.swift) as its own child and talks to it with JSON lines over stdin/stdout.
On Linux (and in the Mac smoke test) `MPB_FAKE_DEVICE=<folder>` replaces the iPhone with a folder (engine/fakedevice.go).

The app can only be built on a Mac: pushing changes under app/, engine/ or VERSION triggers `.github/workflows/build-mac.yml`,
which builds, ad-hoc signs, runs real-Mac smoke tests and force-pushes results to the `ci-results` branch
(this session's proxy blocks the Actions API, so read results via git). Release with `scripts/release.sh`.
In tests use `pkill -x`, never `pkill -f mpbroad-engine`. `signal.Ignore` can't be undone with `signal.Reset`.
Commit author: `Nevertheless_D <NeverthelessD@users.noreply.github.com>`.
