package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/msjurset/anonymark/pkg/anonymizer"
	"github.com/msjurset/anonymark/pkg/renderer"
	"github.com/msjurset/anonymark/pkg/screenshot"
)

const version = "1.2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "capture":
		runCapture(os.Args[2:])
	case "process":
		runProcess(os.Args[2:])
	case "completion":
		runCompletion(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("anonymark version %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`anonymark - Pixel-perfect screenshot anonymizer and synthetic data engine

Usage:
  anonymark <command> [flags]

Commands:
  capture    Capture interactive macOS screenshot region and anonymize
  process    Anonymize an existing image file
  completion Generate shell completion scripts (zsh, bash)
  version    Show anonymark version

Flags for capture & process:
  -out string     Output image file path (default "anonymized.png")
  -mode string    Redaction mode: synthetic, blur, pill (default "synthetic")
`)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func runCapture(args []string) {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	outPath := fs.String("out", "anonymized.png", "Output file path")
	modeStr := fs.String("mode", "synthetic", "Redaction mode (synthetic, blur, pill)")
	_ = fs.Parse(args)

	fmt.Println("[anonymark] Select screen region for capture...")
	tmpPath, err := screenshot.CaptureInteractive()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error capturing screenshot: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(tmpPath)

	anon := anonymizer.NewAnonymizer()
	mode := renderer.Mode(*modeStr)
	resolvedOut := expandPath(*outPath)

	err = anon.ProcessImageFile(tmpPath, resolvedOut, mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error anonymizing image: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[anonymark] Anonymized screenshot saved to %s\n", resolvedOut)
}

func runProcess(args []string) {
	inputPath, outPath, mode := parseProcessArgs(args)

	if inputPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: anonymark process <input-image.png> [-out output.png]\n")
		os.Exit(1)
	}

	anon := anonymizer.NewAnonymizer()
	err := anon.ProcessImageFile(inputPath, outPath, mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error anonymizing image: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[anonymark] Anonymized image saved to %s\n", outPath)
}

func parseProcessArgs(args []string) (inputPath, outputPath string, mode renderer.Mode) {
	outPath := "anonymized.png"
	modeStr := "synthetic"
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-out" || arg == "--out" {
			if i+1 < len(args) {
				outPath = args[i+1]
				i++
			}
		} else if strings.HasPrefix(arg, "-out=") || strings.HasPrefix(arg, "--out=") {
			parts := strings.SplitN(arg, "=", 2)
			outPath = parts[1]
		} else if arg == "-mode" || arg == "--mode" {
			if i+1 < len(args) {
				modeStr = args[i+1]
				i++
			}
		} else if strings.HasPrefix(arg, "-mode=") || strings.HasPrefix(arg, "--mode=") {
			parts := strings.SplitN(arg, "=", 2)
			modeStr = parts[1]
		} else {
			positional = append(positional, arg)
		}
	}

	if len(positional) > 0 {
		inputPath = expandPath(positional[0])
	}
	return inputPath, expandPath(outPath), renderer.Mode(modeStr)
}

func runCompletion(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: anonymark completion <zsh|bash>")
		return
	}

	switch args[0] {
	case "zsh":
		fmt.Println(`#compdef anonymark

_anonymark() {
    local -a commands
    commands=(
        'capture:Capture interactive macOS screenshot region and anonymize'
        'process:Anonymize an existing image file'
        'completion:Generate shell completion scripts'
        'version:Show version'
    )
    _arguments -C \
        '1: :->cmd' \
        '*:: :->args'

    case $state in
        cmd)
            _describe -t commands 'anonymark command' commands
            ;;
        args)
            case $words[1] in
                process)
                    _files -g '*.png'
                    ;;
                capture)
                    _arguments '-out[Output path]:file:_files' '-mode[Redaction mode]:(synthetic blur pill)'
                    ;;
            esac
            ;;
    esac
}

_anonymark "$@"`)
	case "bash":
		fmt.Println(`_anonymark_completions() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="capture process completion version"

    if [[ ${COMP_CWORD} -eq 1 ]] ; then
        COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
        return 0
    fi
}
complete -F _anonymark_completions anonymark`)
	}
}
