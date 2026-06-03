# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.2] - 2026-06-03

### Fixed

- Chat auto-scroll robustness: the previous fix did not fully release the viewport while an agent was streaming. Reworked it into a pin-to-bottom model that distinguishes the app's own programmatic scroll from the user's, so scrolling up during streaming now reliably keeps the viewport in place and auto-scroll only re-engages when the user returns to the bottom or sends a new message.

## [0.1.1] - 2026-06-03

### Fixed

- Chat auto-scroll no longer traps the viewport at the bottom while an agent is streaming a response. Scrolling up now reliably yields control (detected via wheel/touch events), and auto-scroll only re-engages when the user returns to the bottom.

## [0.1.0] - 2026-06-03

### Added

- Initial public release.
