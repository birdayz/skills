package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/genai"
)

func main() {
	md := flag.String("md", "", "Path to prompt markdown file (required)")
	refs := flag.String("refs", "", "Comma-separated reference image paths (required)")
	output := flag.String("output", "output.png", "Output filename (saved in same directory as markdown)")
	model := flag.String("model", "gemini-3-pro-image-preview", "Gemini model to use")
	imageSize := flag.String("size", "4K", "Image size: 512px, 1K, 2K, or 4K (512px is 3.1-flash only)")
	flag.Parse()

	if *md == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --md is required")
		flag.Usage()
		os.Exit(1)
	}
	if *refs == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --refs is required")
		flag.Usage()
		os.Exit(1)
	}

	// Read markdown file as prompt
	mdBytes, err := os.ReadFile(*md)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to read markdown: %v\n", err)
		os.Exit(1)
	}
	mdDir := filepath.Dir(*md)

	// Parse and validate ref paths
	refPaths := strings.Split(*refs, ",")
	for i := range refPaths {
		refPaths[i] = strings.TrimSpace(refPaths[i])
	}
	if len(refPaths) < 1 || len(refPaths) > 14 {
		fmt.Fprintf(os.Stderr, "ERROR: Need 1-14 reference images, got %d\n", len(refPaths))
		os.Exit(1)
	}
	for _, p := range refPaths {
		if _, err := os.Stat(p); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Reference image not found: %s\n", p)
			os.Exit(1)
		}
	}

	// Build prompt: full markdown minus ## QA Requirements section (that's for post-generation)
	prompt := stripQASection(string(mdBytes))

	// Prepend and append no-letterbox instruction
	prompt = "FILL ENTIRE 16:9 FRAME. NO BLACK BARS. NO LETTERBOXING.\n\n" + prompt + "\n\nFILL ENTIRE 16:9 FRAME. NO BLACK BARS. NO LETTERBOXING."

	// Output path
	outputPath := filepath.Join(mdDir, *output)

	fmt.Fprintf(os.Stderr, "Using model: %s\n", *model)
	fmt.Fprintf(os.Stderr, "Markdown: %s\n", *md)
	fmt.Fprintf(os.Stderr, "Loading %d reference images...\n", len(refPaths))

	ctx := context.Background()

	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Build contents: prompt text first, then all reference images
	var contents []*genai.Content
	contents = append(contents, genai.NewContentFromText(prompt, genai.RoleUser))

	for i, refPath := range refPaths {
		fmt.Fprintf(os.Stderr, "  [%d/%d] Loading %s...\n", i+1, len(refPaths), filepath.Base(refPath))

		imageData, err := os.ReadFile(refPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to read %s: %v\n", refPath, err)
			os.Exit(1)
		}

		ext := strings.ToLower(filepath.Ext(refPath))
		mimeType := "image/jpeg"
		if ext == ".png" {
			mimeType = "image/png"
		}

		contents = append(contents, genai.NewContentFromBytes(imageData, mimeType, genai.RoleUser))
	}

	fmt.Fprintln(os.Stderr, "\nGenerating image...")

	config := &genai.GenerateContentConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
		ImageConfig: &genai.ImageConfig{
			AspectRatio: "16:9",
			ImageSize:   *imageSize,
		},
	}

	resp, err := client.Models.GenerateContent(ctx, *model, contents, config)
	if err != nil {
		log.Fatal(err)
	}

	if resp.Candidates == nil || len(resp.Candidates) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: No candidates in response")
		os.Exit(1)
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: No parts in candidate content")
		os.Exit(1)
	}

	var savedImage bool
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			if part.Thought {
				fmt.Fprintf(os.Stderr, "[Model thinking]: %s\n", part.Text)
			} else {
				fmt.Fprintf(os.Stderr, "[Model]: %s\n", part.Text)
			}
		}

		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			err := os.WriteFile(outputPath, part.InlineData.Data, 0644)
			if err != nil {
				log.Fatal(err)
			}
			savedImage = true
			fmt.Fprintf(os.Stderr, "  Image size: %d bytes\n", len(part.InlineData.Data))
		}
	}

	if !savedImage {
		fmt.Fprintln(os.Stderr, "ERROR: No image data found in response")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "\n✓ Image generated successfully!")
	fmt.Fprintf(os.Stderr, "  Saved to: %s\n", outputPath)

	// Print token usage and cost estimate
	if u := resp.UsageMetadata; u != nil {
		fmt.Fprintln(os.Stderr, "\n--- Token Usage ---")
		fmt.Fprintf(os.Stderr, "  Prompt tokens:    %d\n", u.PromptTokenCount)
		fmt.Fprintf(os.Stderr, "  Candidate tokens: %d\n", u.CandidatesTokenCount)
		if u.ThoughtsTokenCount > 0 {
			fmt.Fprintf(os.Stderr, "  Thinking tokens:  %d\n", u.ThoughtsTokenCount)
		}
		fmt.Fprintf(os.Stderr, "  Total tokens:     %d\n", u.TotalTokenCount)

		for _, d := range u.PromptTokensDetails {
			fmt.Fprintf(os.Stderr, "    input  %-8s %d tokens\n", d.Modality, d.TokenCount)
		}
		for _, d := range u.CandidatesTokensDetails {
			fmt.Fprintf(os.Stderr, "    output %-8s %d tokens\n", d.Modality, d.TokenCount)
		}

		cost := estimateCost(*model, u)
		fmt.Fprintf(os.Stderr, "\n--- Cost Estimate ---\n")
		fmt.Fprintf(os.Stderr, "  Input:  $%.4f\n", cost.input)
		fmt.Fprintf(os.Stderr, "  Output: $%.4f\n", cost.output)
		fmt.Fprintf(os.Stderr, "  Total:  $%.4f\n", cost.input+cost.output)
	}

	fmt.Fprintf(os.Stderr, "\nTo view: xdg-open %s\n", outputPath)
}

type costEstimate struct {
	input  float64
	output float64
}

// estimateCost calculates USD cost based on Gemini API pricing.
// Pricing source: https://ai.google.dev/gemini-api/docs/pricing
func estimateCost(model string, u *genai.GenerateContentResponseUsageMetadata) costEstimate {
	// Pricing per 1M tokens. Source: https://ai.google.dev/gemini-api/docs/pricing
	// Input text+image share the same rate. Output text and image have different rates.
	inputRate := 2.0        // $/1M input tokens (text + image)
	outputTextRate := 12.0  // $/1M output text tokens
	outputImageRate := 120.0 // $/1M output image tokens

	switch {
	case strings.Contains(model, "3.1-flash"):
		// Nano Banana 2: gemini-3.1-flash-image-preview
		// https://ai.google.dev/gemini-api/docs/pricing
		inputRate = 0.25
		outputTextRate = 1.50
		outputImageRate = 60.0
	case strings.Contains(model, "3-pro"):
		// Nano Banana Pro: gemini-3-pro-image-preview
		inputRate = 2.0
		outputTextRate = 12.0
		outputImageRate = 120.0
	case strings.Contains(model, "2.5-pro"):
		inputRate = 1.25
		outputTextRate = 10.0
		outputImageRate = 100.0
	case strings.Contains(model, "2.5-flash"):
		inputRate = 0.15
		outputTextRate = 0.60
		outputImageRate = 6.0
	case strings.Contains(model, "2.0-flash"):
		inputRate = 0.10
		outputTextRate = 0.40
		outputImageRate = 4.0
	}

	c := costEstimate{
		input: float64(u.PromptTokenCount) / 1_000_000 * inputRate,
	}
	for _, d := range u.CandidatesTokensDetails {
		tokens := float64(d.TokenCount) / 1_000_000
		if d.Modality == "IMAGE" {
			c.output += tokens * outputImageRate
		} else {
			c.output += tokens * outputTextRate
		}
	}
	return c
}

// stripQASection removes the ## QA Requirements section and everything after it.
// QA is for post-generation validation, not for the generation prompt.
func stripQASection(mdText string) string {
	idx := strings.Index(mdText, "\n## QA Requirements")
	if idx == -1 {
		idx = strings.Index(mdText, "\n## QA")
	}
	if idx != -1 {
		return strings.TrimSpace(mdText[:idx])
	}
	return mdText
}
