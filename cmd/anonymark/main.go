package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/msjurset/anonymark/pkg/anonymizer"
	"github.com/msjurset/anonymark/pkg/renderer"
	"github.com/msjurset/anonymark/pkg/screenshot"
)

const version = "1.0.0"

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

	err = anon.ProcessImageFile(tmpPath, *outPath, mode, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error anonymizing image: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[anonymark] Anonymized screenshot saved to %s\n", *outPath)
}

func runProcess(args []string) {
	fs := flag.NewFlagSet("process", flag.ExitOnError)
	outPath := fs.String("out", "anonymized.png", "Output file path")
	modeStr := fs.String("mode", "synthetic", "Redaction mode (synthetic, blur, pill)")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: anonymark process <input-image.png> [-out output.png]\n")
		os.Exit(1)
	}

	inputPath := fs.Arg(0)
	anon := anonymizer.NewAnonymizer()
	mode := renderer.Mode(*modeStr)

	err := anon.ProcessImageFile(inputPath, *outPath, mode, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error anonymizing image: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[anonymark] Anonymized image saved to %s\n", *outPath)
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
