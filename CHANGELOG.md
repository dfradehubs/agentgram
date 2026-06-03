# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.3] - 2026-06-03

### Fixed

- Streaming responses now render incrementally (token by token) again. Next.js gzip compression was buffering `text/event-stream` responses and flushing them in large blocks, so the agent's reply only appeared once it was fully written. Compression is now disabled in Next (`compress: false`); static asset compression should be handled at the edge (ingress/CDN/service mesh).
- Reloading the page during an active run no longer fires a duplicate run. The backend keeps the run alive and persists the full reply when it finishes, so the client now polls the session for that reply and shows it once the run completes, only re-sending as a last resort if nothing arrives.

## [0.1.2] - 2026-06-03

### Fixed

- Chat auto-scroll robustness: the previous fix did not fully release the viewport while an agent was streaming. Reworked it into a pin-to-bottom model that distinguishes the app's own programmatic scroll from the user's, so scrolling up during streaming now reliably keeps the viewport in place and auto-scroll only re-engages when the user returns to the bottom or sends a new message.

## [0.1.1] - 2026-06-03

### Fixed

- Chat auto-scroll no longer traps the viewport at the bottom while an agent is streaming a response. Scrolling up now reliably yields control (detected via wheel/touch events), and auto-scroll only re-engages when the user returns to the bottom.

## [0.1.0] - 2026-06-03

### Added

- Initial public release.
