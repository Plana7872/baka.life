package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	registrydiff "github.com/NyanLoli-Network/baka.life/registryctl/diff"
	"github.com/NyanLoli-Network/baka.life/registryctl/parser"
	"github.com/NyanLoli-Network/baka.life/registryctl/provider/cloudflare"
	"github.com/NyanLoli-Network/baka.life/registryctl/registry"
	"github.com/NyanLoli-Network/baka.life/registryctl/review"
	registrysync "github.com/NyanLoli-Network/baka.life/registryctl/sync"
	"github.com/NyanLoli-Network/baka.life/registryctl/validator"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: baka-registry <validate|review-check|diff|sync> [flags]")
	}

	switch args[0] {
	case "validate":
		return runValidate(args[1:])
	case "review-check":
		return runReviewCheck(args[1:])
	case "diff":
		return runDiff(args[1:])
	case "sync":
		return runSync(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", ".", "repository root")
	githubAuthor := fs.String("github-author", os.Getenv("GITHUB_PR_AUTHOR"), "GitHub PR author to authorize")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reg, err := loadRegistry(*root)
	if err != nil {
		return err
	}
	if err := validator.Validate(reg); err != nil {
		return err
	}
	if *githubAuthor != "" {
		if err := validator.AuthorizeGitHub(reg, *githubAuthor); err != nil {
			return err
		}
	}

	fmt.Println("registry validation passed")
	return nil
}

func runReviewCheck(args []string) error {
	fs := flag.NewFlagSet("review-check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	baseRoot := fs.String("base-root", "", "base repository root")
	headRoot := fs.String("head-root", ".", "PR repository root")
	githubAuthor := fs.String("github-author", os.Getenv("GITHUB_PR_AUTHOR"), "GitHub PR author")
	githubOutput := fs.String("github-output", os.Getenv("GITHUB_OUTPUT"), "GitHub Actions output file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baseRoot == "" {
		return fmt.Errorf("-base-root is required")
	}
	if *githubAuthor == "" {
		return fmt.Errorf("-github-author is required")
	}

	result, err := review.Check(*baseRoot, *headRoot, *githubAuthor)
	if err != nil {
		return err
	}

	if err := writeReviewOutputs(*githubOutput, result); err != nil {
		return err
	}

	fmt.Printf("registered=%t\n", result.Registered)
	fmt.Printf("requires_review=%t\n", result.RequiresReview)
	fmt.Printf("auto_merge=%t\n", result.AutoMerge)
	for _, file := range result.ChangedFiles {
		fmt.Println("changed: " + file)
	}
	for _, reason := range result.Reasons {
		fmt.Println("review: " + reason)
	}
	return nil
}

func runDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reg, err := loadRegistry(*root)
	if err != nil {
		return err
	}
	if err := validator.Validate(reg); err != nil {
		return err
	}
	desired, err := reg.DesiredRecords()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cf, err := cloudflare.NewFromEnv()
	if err != nil {
		return err
	}
	current, err := cf.ListRecords(ctx)
	if err != nil {
		return err
	}
	changes := registrydiff.Generate(current, desired)
	printChanges(changes)
	return nil
}

func runSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reg, err := loadRegistry(*root)
	if err != nil {
		return err
	}
	if err := validator.Validate(reg); err != nil {
		return err
	}
	desired, err := reg.DesiredRecords()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cf, err := cloudflare.NewFromEnv()
	if err != nil {
		return err
	}
	changes, err := registrysync.Apply(ctx, cf, desired)
	if err != nil {
		printChanges(changes)
		return err
	}
	printChanges(changes)
	return nil
}

func loadRegistry(root string) (*registry.Registry, error) {
	return parser.ParseRegistry(root)
}

func printChanges(changes []registrydiff.Change) {
	if len(changes) == 0 {
		fmt.Println("no changes")
		return
	}
	fmt.Println(registrydiff.FormatChanges(changes))
}

func writeReviewOutputs(path string, result review.Result) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "registered=%t\nrequires_review=%t\nauto_merge=%t\n", result.Registered, result.RequiresReview, result.AutoMerge)
	return err
}
