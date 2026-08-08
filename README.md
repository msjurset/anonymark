# anonymark

**anonymark** is a native Go CLI utility for capturing and anonymizing screenshots with **pixel-perfect synthetic data replacement**. Instead of blurring or redacting sensitive data with black boxes, `anonymark` samples the text color, font size, and background style to replace private IP addresses, MAC addresses, hostnames, user paths, and tokens with realistic synthetic data.

---

## Features

- **Synthetic In-Place Replacement**: Replaces IP addresses (`192.168.86.55` $\rightarrow$ `10.0.4.12`), MAC addresses (`D8:3A:DD:4C:F2:BB` $\rightarrow$ `52:54:00:12:34:56`), hostnames (`pi.hole` $\rightarrow$ `node-alpha.internal`), user paths (`/Users/...`), and tokens in-place using matching colors and geometry.
- **Session-Level Consistency**: Deterministically maps identical sensitive values across multiple screenshots so documentation remains coherent.
- **Interactive macOS Capture**: Launches native macOS selection overlay (`screencapture -i`) directly from terminal.
- **Multi-Mode Support**: Supports `synthetic` (default), `blur`, and `pill` redaction styles.
- **100% Offline & Private**: Zero external network calls or cloud dependencies.

---

## Installation & Build

### Prerequisites
- Go 1.22+ toolchain
- macOS (for interactive `capture` command)

### Build & Install

```bash
# Clone and build binary
git clone https://github.com/msjurset/anonymark.git
cd anonymark
make build

# Run unit test suite
make test

# Install to /usr/local/bin
sudo make install
```

---

## Usage

### Interactive Screen Capture
```bash
# Capture area interactively and save anonymized image to output.png
anonymark capture -out output.png
```

### Anonymize Existing Image
```bash
# Process an existing screenshot PNG file
anonymark process screenshot.png -out anonymized.png -mode synthetic
```

### Redaction Modes
- `-mode synthetic` *(default)*: Replaces sensitive PII with realistic fake data matching background/foreground styles.
- `-mode blur`: Applies pixelated privacy blur over sensitive regions.
- `-mode pill`: Renders clean dark-mode redaction pills over target regions.

---

## Shell Completions

Generate shell completions for Zsh or Bash:
```bash
# Zsh completion
anonymark completion zsh > ~/.zsh/completions/_anonymark

# Bash completion
anonymark completion bash > /etc/bash_completion.d/anonymark
```

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
