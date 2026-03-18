---
name: generate-with-refs
description: Generate images using Gemini with multiple reference images and mandatory QA validation
user-invocable: true
allowed-tools: Bash, Read, Write
argument-hint: <folder-path> [--output <filename>] [--model <model-name>] [--size <512px|1K|2K|4K>]
---

# Generate Image with References

Generate images using Google Gemini API with multiple reference images. This prevents AI hallucinations by providing actual visual references for character designs, environments, and art styles.

**NEW WORKFLOW:** Operates on folders containing structured markdown prompts and reference images, with mandatory QA validation.

## Usage

```
/generate-with-refs <folder-path> [--output <filename>] [--model <model-name>] [--size <1K|2K|4K>]
```

### Arguments

- **folder-path**: Path to folder containing `*-prompt.md` and reference images (required)
- **--output**: Output filename (default: `output.png` in the folder)
- **--model**: Gemini model to use (default: `gemini-3.1-flash-image-preview` for best quality)
- **--size**: Image size: 512px, 1K, 2K, or 4K (default: 4K). Note: 512px is only supported on gemini-3.1-flash-image-preview.

### Examples

```bash
# Generate from folder with structured prompt
/generate-with-refs path/to/project-folder

# Use specific model and size
/generate-with-refs path/to/folder --model gemini-2.5-pro-exp-03 --size 2K

# Custom output filename
/generate-with-refs path/to/folder --output final.png
```

## Folder Structure

The folder must contain:

1. **Prompt markdown file** (name must end with `-prompt.md`):
   - `## Reference Images` - List of reference image filenames
   - `## Technical Specifications` - Aspect ratio, resolution, style
   - `## Layout & Composition` - Positioning and composition guidelines
   - `## Environment` - Scene description and rendering quality
   - `## Characters` (or other content sections) - Detailed descriptions with references
   - `## Critical Requirements` - Quality and fidelity mandates
   - `## QA Requirements` (optional) - QA validation criteria

2. **Reference images** - Image files listed in the markdown

3. **Output image** - Generated in the same folder (default: `output.png`)

## Instructions

### Step 1: Find Markdown File

1. Check `$ARGUMENTS` for folder path and optional flags
2. Find prompt markdown file in folder (`*-prompt.md`)
3. Read the markdown to extract QA requirements for post-generation validation

### Step 2: Build Refs List

Read the `## Reference Images` section from the markdown. Build comma-separated absolute paths to all ref images in the folder.

### Step 3: Generate Image

```bash
cd <skill-dir>/scripts  # <skill-dir> = directory containing this SKILL.md

go run generate.go \
  --md "/absolute/path/to/folder/thumbnail-prompt.md" \
  --refs "/absolute/path/to/folder/ref1.jpg,/absolute/path/to/folder/ref2.jpg,..." \
  --output "output.png" \
  --model "gemini-3.1-flash-image-preview" \
  --size "4K"
```

The script:
- `--md`: Reads markdown file as the prompt (strips `## QA Requirements` section automatically)
- `--refs`: Comma-separated absolute paths to reference images (2-14 required)
- `--output`: Filename saved in same directory as the markdown (default: `output.png`)
- Prepends/appends "NO BLACK BARS" instruction automatically

### Step 5: Mandatory QA Pass

**CRITICAL:** Always perform QA validation after generation.

1. Read the generated image
2. Read all reference images
3. Perform visual comparison:
   - Check if elements match reference designs EXACTLY
   - Verify quality matches the specified style (e.g., photorealistic, stylized, etc.)
   - Validate layout matches requirements
   - Check for hallucinations or invented elements
   - Verify color accuracy and materials

4. If `## QA Requirements` section exists in markdown, validate against those specific requirements

5. Report QA results:
   - ✅ PASS: Element X matches reference
   - ❌ FAIL: Element Y is hallucinated/incorrect
   - ⚠️ ISSUE: Layout doesn't match requirements

### Step 6: Report Results

1. Show QA verdict (PASS/FAIL)
2. List any issues found
3. Show generated image path
4. Provide xdg-open command for viewing
5. If QA FAIL: Prompt user whether to regenerate or edit prompt

## Reference Image Guidelines

**Best practices:**
- Use 2-14 reference images (Gemini supports up to 6 objects + 5 humans)
- Provide clear examples of characters, environments, and styles
- Use high-quality source images (avoid compressed/low-res)
- Mix object references (environments, items) and character references

**What to avoid:**
- Generic stock photos (use actual source material specific to your project)
- Conflicting art styles across references
- Too many references (diminishing returns after ~6)

## Environment Variables

- **GEMINI_API_KEY** or **GOOGLE_API_KEY**: Required for Gemini API access
- Get free key from: https://aistudio.google.com/apikey

## Generating the Final Image (Approved Iteration → 2K)

When the user approves an iteration, **do NOT simply re-run the original prompt at 2K** — the model will generate a different result from scratch. Instead, use the approved iteration as the sole reference with a minimal "upscale" prompt:

### Step 1: Create a minimal upscale prompt

Create `upscale-prompt.md` in the thumbnail folder:

```markdown
# Upscale <iteration-N>

## Reference Images

1. `iteration-N.png` — **SOURCE**: Reproduce this image exactly at higher resolution. Same composition, same characters, same colors, same poses, same everything. Do not change anything.

## Technical Specifications

- **Aspect Ratio:** 16:9 widescreen, full frame — NO black bars
- **Style:** Match the source image exactly

## Instructions

Reproduce `iteration-N.png` at 2K resolution. Keep every detail identical:
- Same character positions and sizes
- Same colors and lighting
- Same composition
- Same art style

Do not add, remove, or change anything. This is a pure upscale.
```

### Step 2: Generate with only the approved iteration as reference

```bash
go run generate.go \
  --md "/path/to/thumbnail/upscale-prompt.md" \
  --refs "/path/to/thumbnail/iteration-N.png" \
  --output "thumbnail.png" \
  --model "gemini-3-pro-image-preview" \
  --size "2K"
```

**Key points:**
- Only 1 reference image — the approved iteration. No game refs, no other images.
- Minimal prompt — just "reproduce this exactly." No character descriptions, no layout instructions.
- The less the model has to interpret, the closer the output matches the approved iteration.
- Use `gemini-3-pro-image-preview` (not flash) for final quality.

## Notes

- Uses Go Gemini SDK for native performance
- Outputs IMAGE modality (not text+image)
- Uses multimodal understanding to blend reference styles
- Works with Gemini 2.0+ models that support image generation
- **Model for iterations:** `gemini-3.1-flash-image-preview` (fast, cheap, supports 512px)
- **Model for final:** `gemini-3-pro-image-preview` (higher quality, thinking model)
