# Docket

Docket is a Go terminal document classifier built around [TypeSafe's Jev](https://typesafe.ai/blog/introducing-system-one-models-and-jev). It copies a document into a local library, extracts its text, asks Jev for typed classification decisions, and files the copy without moving or editing the source.

## How Docket uses Jev

[Jev](https://docs.typesafe.ai/concepts/system-one) is TypeSafe's first System One model. Instead of generating a paragraph and making the application parse it, Jev answers bounded questions with typed values, probabilities, and confidence. Docket asks Jev four questions about every document:

| Jev primitive | Question | Result used by Docket |
| --- | --- | --- |
| `Choice` | What is the document's primary category? | Category, probability distribution, and confidence |
| `Score` | How sensitive is its information? | A score from 0 to 3 and the nearest handling level |
| `Score` | How urgently must the recipient respond? | A score from 0 to 3 and the nearest urgency level |
| `Noul` | Does the document require the recipient to act? | Probability that a response, payment, signature, submission, attendance, or other action is required |

Docket keeps those Jev results visible and puts deterministic Go logic around them:

```text
JEV CLASSIFICATION
Category       HOUSING
Confidence     94%

JEV OUTPUT
Sensitivity    0.8 / 3 · Personal
Urgency        1.6 / 3 · Time-sensitive
Needs action   82%

DOCKET GUIDANCE
Next step      Act soon; check exact deadline
Handling       Keep private
Action         Likely required · 82%
Review         Not required
```

Go applies explicit thresholds to flag uncertain categories for review, translate scores into practical labels, choose the next step, and select the filing destination. Jev does not run OCR, copy files, or choose filesystem paths. It receives extracted text and provides the judgments that drive the workflow.

| Stage | Implementation | Result |
| --- | --- | --- |
| Copy | Local Go code | A managed copy; the source remains untouched |
| Read | Local parsing for text and Markdown, or the selected OpenAI/OpenRouter model for PDFs and images | Extracted text |
| Decide | Jev | Typed category, sensitivity, urgency, and action judgments |
| File | Local Go code and SQLite | Searchable metadata and a category folder |

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
  docket models [provider]       List live PDF and image extraction models
```

## Install

Docket requires Go 1.23 or later.

```sh
go install github.com/glamboyosa/docket/cmd/docket@latest
```

The binary does not modify source documents. During use, Docket creates a SQLite index and a managed document library. See [Local data and credentials](#local-data-and-credentials) for their locations.

## Configure credentials

Classification requires a [TypeSafe](https://typesafe.ai/) API key. PDF and image extraction also requires an OpenAI or OpenRouter API key. Plain-text and Markdown files are read locally and do not require an extraction-provider key.

```sh
docket auth set typesafe
docket auth set openrouter
```

Keys are read without echoing and stored in the operating system keychain under the `docket` service. They are never written to `config.json` or SQLite. Environment variables take precedence over the keychain and can be used instead:

| Service | Environment variable |
| --- | --- |
| TypeSafe | `TYPESAFE_API_KEY` |
| OpenRouter | `OPENROUTER_API_KEY` |
| OpenAI | `OPENAI_API_KEY` |

Check which credential source Docket will use:

```sh
docket auth status
```

## Commands

| Command | What it does |
| --- | --- |
| `docket` | Open the terminal interface |
| `docket add <path> [path…]` | Import one or more files or directories |
| `docket add -` | Import text from standard input |
| `docket auth status` | Show whether each credential comes from the environment, keychain, or is missing |
| `docket auth set <provider>` | Read a TypeSafe, OpenAI, or OpenRouter key and save it to the OS keychain |
| `docket auth forget <provider>` | Remove a saved key from the OS keychain |
| `docket config` | Show the extraction provider, model, and library path |
| `docket config provider <provider>` | Select `openai` or `openrouter` |
| `docket config model <model-id>` | Select the extraction model |
| `docket config library <path>` | Change the managed library location |
| `docket models [provider]` | List live models that support both PDF and image extraction |
| `docket help` | Show command-line help |

### Examples

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

`docket models` checks the provider's live catalog and uses [Models.dev](https://models.dev/) metadata to keep only models that accept both PDFs and images and return text. OpenAI results are also limited to models available to the configured API key.

### Terminal interface keys

| Key | Action |
| --- | --- |
| `a` | Enter a file or folder path to import |
| `/` | Filter documents by filename or category |
| `↑` / `k` | Move to the previous document or option |
| `↓` / `j` | Move to the next document or option |
| `Enter` | Inspect the selected document and its Jev guidance |
| `s` | Open provider and model settings |
| `Enter` / `Ctrl+L` | Browse live compatible models while settings are open |
| `Ctrl+S` | Save a manually entered model ID |
| `Cmd+V` | Paste a file or folder path in the add-documents panel on macOS |
| `r` | Refresh the document library |
| `?` | Open keyboard help |
| `Esc` | Close the current panel or cancel input |
| `q` | Quit |

## Supported documents

Docket accepts files up to 25 MB:

- PDF
- PNG
- JPEG
- WebP
- plain text
- Markdown

Jev classifies documents into `tax`, `legal`, `financial`, `medical`, `identity`, `insurance`, `employment`, `education`, `housing`, `receipts`, `correspondence`, or `other`. Low-confidence classifications are marked for review. Jev's results are triage signals, so check the document itself for exact dates and obligations.

## Local data and credentials

Docket stores application data in `~/Library/Application Support/docket` on macOS, `${XDG_CONFIG_HOME:-~/.config}/docket` on Linux, or `%AppData%\docket` on Windows. Set `DOCKET_HOME` to use a different directory.

| Local resource | What it contains |
| --- | --- |
| `config.json` | The selected extraction provider, model ID, and managed library path. It contains no API keys or document text. |
| `docket.db` | Document name, SHA-256 hash, source and managed-copy paths, processing status, extraction provider and model, Jev category probabilities and confidence, sensitivity, urgency, action probability, review flag, errors, and timestamps. It contains no API keys or extracted document text. |
| OS keychain | The TypeSafe, OpenRouter, and OpenAI keys saved with `docket auth set`, under the `docket` service. |
| Managed library | Copies of imported documents, filed by Jev category. The default path is `~/Documents/Docket`. |

The configuration file is ordinary local JSON. A default OpenRouter configuration looks like this:

```json
{
  "provider": "openrouter",
  "model": "openrouter/auto",
  "library_path": "/Users/you/Documents/Docket"
}
```

Docket creates its data directory with user-only permissions and writes `config.json` with mode `0600` where the operating system supports Unix file modes. Credentials supplied through environment variables are not copied into the keychain.

## Network access

| Service | Data sent | Result |
| --- | --- | --- |
| OpenAI or OpenRouter | The PDF or image bytes encoded as a data URL and a transcription instruction. PDF requests include the filename. Plain-text and Markdown files skip this request. | Extracted text, held in memory and not stored by Docket |
| TypeSafe | The extracted text, the four Jev questions and their criteria, and the `jev-latest` model identifier | Category probabilities and confidence, sensitivity and urgency scores, score confidence, and action probability. Docket stores the classification results in SQLite. |
| OpenAI or OpenRouter model catalog | A request to list available models. OpenAI catalog requests use the configured OpenAI key. No document is sent. | Model identifiers and capability metadata |
| Models.dev | A request for public model capability and release metadata. No document or API key is sent. | Public model metadata used to filter the provider catalog |

Extracted text exists in memory long enough to send it to Jev. Docket does not write it to `config.json`, SQLite, or a separate text file. The source document is read to make the managed copy but is never moved, renamed, or edited.

## Remove Docket

Remove saved credentials first:

```sh
docket auth forget typesafe
docket auth forget openrouter
docket auth forget openai
```

Then remove the `docket` binary from the Go binary directory. Delete the Docket user-config directory to remove the SQLite index and configuration. Delete the managed library separately if its copied documents are no longer needed. Source documents are not affected.

If credentials were supplied through environment variables, unset them in the shell or profile where they were configured.

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

## Testing

The normal test suite uses fake HTTP transports. It checks provider payloads and responses without using API credits:

```sh
go test ./...
```

Download the public PDF fixtures from IRS, USCIS, CFPB, and CMS:

```sh
./scripts/download-test-documents.sh
```

Run the document corpus against one provider or both:

```sh
./scripts/test-live.sh openai
./scripts/test-live.sh openrouter
./scripts/test-live.sh all
```

Live tests load credentials from `.env`, send PDFs and images to the selected extraction provider, and send extracted text to TypeSafe. Each run uses a temporary Docket configuration, database, and library, then removes them. Downloaded PDFs remain under `testdata/documents/downloaded` for manual testing and are ignored by Git.

Set `DOCKET_TEST_FILTER` to run matching cases only:

```sh
DOCKET_TEST_FILTER=form-w9 ./scripts/test-live.sh all
```

Override the default test models with `DOCKET_TEST_OPENAI_MODEL` or `DOCKET_TEST_OPENROUTER_MODEL`. Cases and accepted categories are listed in `testdata/cases.tsv`.
