# Roadmap

Docket's next work is focused on making document imports easier without weakening its copy-first handling or local storage. The sections below show order, not promised release dates.

## Available now

- [x] Import files, folders, or pasted text from the terminal.
- [x] Read plain text and Markdown locally.
- [x] Read PDFs and images with a user-selected OpenAI or OpenRouter model.
- [x] Use Jev to classify each document and estimate sensitivity, urgency, and whether it needs action.
- [x] Keep source files untouched and file managed copies by category.
- [x] Store API keys in the operating system keychain and document records in local SQLite.
- [x] Browse compatible extraction models from live provider catalogs.
- [x] Publish release binaries for macOS, Linux, and Windows.

## Next

- [ ] Add a graphical file picker to the terminal interface. Selected files will use the existing copy-first import flow.
- [ ] Add opt-in watched folders. Docket will import new supported files while leaving the originals where they are.
- [ ] Add a PowerShell installer so Windows users can install a release binary without Go.

## Later

- [ ] Add optional local OCR for PDFs and images, with no provider API key or document upload.
- [ ] Let users define their own categories and destination folders while keeping Jev's confidence and review behavior visible.

## Constraints

Planned work must preserve these behaviors:

- Docket does not move, rename, or edit source documents.
- API keys do not go into JSON or SQLite.
- Extracted document text is not stored after classification.
- Uncertain classifications remain visible for review.

Requests and bug reports belong in [GitHub Issues](https://github.com/glamboyosa/docket/issues).
