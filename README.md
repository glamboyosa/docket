# Docket

Docket is a Go terminal application that copies, reads, classifies, and files documents into a local library.

```console
$ docket help
Docket copies, reads, classifies, and files your documents.

Usage:
  docket                         Open the terminal interface
  docket add <path> [path…]      Import files or directories
  docket add -                   Import pasted text from stdin
  docket auth status             Show credential sources
  docket auth set <provider>     Save a key to the OS keychain
  docket auth forget <provider>  Remove a key from the OS keychain
  docket config                  Show extraction settings
  docket config <key> <value>    Set provider, model, or library
  docket models [provider]       List attachment models from Models.dev
```

## Install

Docket requires Go 1.23 or later.

```sh
go install github.com/glamboyosa/docket/cmd/docket@latest
```

The binary does not modify source documents. During use, Docket creates a SQLite index and a managed document library. See [Files and network access](#files-and-network-access) for their locations.

## Configure credentials

Classification requires a [TypeSafe](https://typesafe.ai/) API key. PDF and image extraction also requires an OpenAI or OpenRouter API key. Plain-text and Markdown files are read locally and do not require an extraction-provider key.

```sh
docket auth set typesafe
docket auth set openrouter
```

Keys are read without echoing and stored in the operating system keychain. Environment variables can be used instead:

| Service | Environment variable |
| --- | --- |
| TypeSafe | `TYPESAFE_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |

Check which credential source Docket will use:

```sh
docket auth status
```

## Usage

Open the terminal interface:

```sh
docket
```

Import one file, several files, or a directory:

```sh
docket add ~/Downloads/lease.pdf
docket add ~/Downloads/invoice.pdf ~/Downloads/receipt.jpg
docket add ~/Downloads/to-file
```

Directory imports recurse into subdirectories. Docket ignores unsupported files and symbolic links. Imports run in path order and stop at the first error; documents completed before the error remain filed.

Import text from standard input:

```sh
pbpaste | docket add -
```

Choose OpenAI instead of the default OpenRouter provider:

```sh
docket auth set openai
docket config provider openai
docket models openai
docket config model <model-id>
```

`docket models` reads [Models.dev](https://models.dev/) and lists models that accept attachments for the selected provider.

## Supported documents

Docket accepts files up to 25 MB:

- PDF
- PNG
- JPEG
- WebP
- plain text
- Markdown

Jev classifies documents into `tax`, `legal`, `financial`, `medical`, `identity`, `insurance`, `employment`, `education`, `housing`, `receipts`, `correspondence`, or `other`. Low-confidence classifications are marked for review.

## Files and network access

| Resource | Behavior |
| --- | --- |
| Source document | Read to make a copy; never moved, renamed, or edited |
| Managed library | `~/Documents/Docket` by default; change with `docket config library <path>` |
| SQLite index and configuration | `~/Library/Application Support/docket` on macOS, `${XDG_CONFIG_HOME:-~/.config}/docket` on Linux, or `%AppData%\docket` on Windows |
| Credentials | Operating system keychain under the `docket` service, unless supplied by environment variable |
| OpenAI or OpenRouter | Receives PDFs and images for transcription using the configured model |
| TypeSafe | Receives extracted document text for Jev classification |
| Models.dev | Queried only when listing models |

Extracted text is not stored. The SQLite index contains file paths, hashes, provider and model identifiers, classification results, status, and errors.

## Remove Docket

Remove saved credentials first:

```sh
docket auth forget typesafe
docket auth forget openrouter
docket auth forget openai
```

Then remove the `docket` binary from the Go binary directory. Delete the Docket user-config directory to remove the SQLite index and configuration. Delete the managed library separately if its copied documents are no longer needed. Source documents are not affected.

## Development

Run the Go checks:

```sh
go test ./...
go vet ./...
go build ./cmd/docket
```

Run the marketing site:

```sh
cd site
pnpm install
pnpm dev
```

Check and build the site with `pnpm check` and `pnpm build`.
