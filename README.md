# anonymark

**anonymark** is a native Go and macOS Vision/CoreText CLI utility for capturing and anonymizing screenshots with **pixel-matched synthetic data replacement**. Instead of obscuring UI layouts with black boxes or coarse pixelation, `anonymark` samples the surrounding background color cluster, measures glyph ink height and font weight, and renders realistic synthetic replacements for private IPv4 addresses, MAC addresses, hostnames, user paths, and tokens directly on the original typographic baseline.

![Anonymark Synthetic In-Place Replacement](docs/assets/synthetic_replacement.png)

---

## Documentation

- **[User Guide & Visual Walkthrough](docs/USER_GUIDE.md)** — Architecture overview, clustered background sampling, CoreText baseline matching, and CLI reference.
- **[Practical Cookbook & Recipes](docs/COOKBOOK.md)** — Step-by-step recipes for interactive capture, batch sanitization, compliance pill redaction, and clipboard automation.

---

## Features

- **Pixel-Matched Synthetic Replacement (`-mode synthetic`)**: Replaces private IPv4 addresses, MAC addresses, `.lan`/`.local` hostnames, device names (`nas-*`, `pve-*`, `dns-*`, `iot-*`), `/Users/<name>` paths, emails, and API tokens in-place using CoreText and SF Pro / SF Mono font metrics.
- **Clustered Perimeter Background Sampling**: Samples the 2px exterior ring around each target observation and selects the dominant color cluster so erased regions blend seamlessly into dark-mode cards and selection highlights.
- **Column Snapping & Width Clamping**: Aligns vertically stacked list items to a shared left X coordinate and point size, and clamps replacement width so synthetic text never collides with adjacent UI badges.
- **Session-Level Determinism**: Identical sensitive values map to identical synthetic replacements across the image.
- **Interactive macOS Capture (`anonymark capture`)**: Launches the native macOS region selection overlay (`screencapture -i`) directly from your terminal.
- **Multi-Mode Support**: Supports `synthetic` (default), `pill`, and `blur` redaction styles.
- **100% Offline & Local**: Runs entirely on-device using macOS Vision and CoreText frameworks with zero external network calls.

---

## Installation & Build

```bash
# Build binary
make build

# Run unit test suite
make test

# Install to /usr/local/bin
sudo make install
```

---

## Quick Start

```bash
# Capture screen region interactively and save sanitized screenshot
anonymark capture -out output.png

# Anonymize an existing PNG screenshot in-place
anonymark process screenshot.png -out anonymized.png -mode synthetic

# Redact sensitive values with solid dark-mode pills
anonymark process screenshot.png -out redacted.png -mode pill
```

---

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
