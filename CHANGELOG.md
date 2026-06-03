# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.1] - 2026-06-03

### Fixed

- Chat auto-scroll no longer traps the viewport at the bottom while an agent is streaming a response. Scrolling up now reliably yields control (detected via wheel/touch events), and auto-scroll only re-engages when the user returns to the bottom.

## [0.1.0] - 2026-06-03

### Added

- Initial public release.
