# birdayz-skills

Claude Code skills. Image generation, thumbnails, that sort of thing.

## Install

### In Claude Code

```
/plugin marketplace add birdayz/skills
/plugin install image-generation@birdayz-skills
```

### CLI

```
claude plugin marketplace add https://github.com/birdayz/skills
claude plugin install image-generation@birdayz-skills
```

## Skills

### generate-with-refs

Generate images with Google Gemini using reference images. Prevents hallucinations by grounding generation in actual visual references.

The key idea: generation is driven by a markdown spec file (`*-prompt.md`), not chat prompts. When the AI needs to change something, it edits the spec. This is 100x more effective than wildly prompting in chat — you get reproducible, iteratable results.

```
/generate-with-refs path/to/folder
```

The folder needs a `*-prompt.md` and reference images. See the [SKILL.md](generate-with-refs/SKILL.md) for details.

Requires `GEMINI_API_KEY` or `GOOGLE_API_KEY` env var and Go toolchain.

## License

MIT
