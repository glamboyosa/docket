# Docket tracing and evals

Set `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY`, and `LANGFUSE_BASE_URL` in the shell to send traces to Langfuse. Docket traces each document as `docket.process`, with child extraction and classification observations. Traces include provider, model, timing, status, category, confidence, and action flag. They exclude document text, file contents, paths, and API keys. Tracing is off when either key is absent.

Run the synthetic live evaluation with `./scripts/test-live.sh openai`, `./scripts/test-live.sh openrouter`, or `./scripts/test-live.sh all`. The script loads `.env`, uses temporary state, and reports category and action accuracy. When Langfuse keys are set, it attaches `docket.category_accuracy` and `docket.action_accuracy` scores to each test trace. Set `DOCKET_TEST_FILTER` to run a subset. These calls use real provider and TypeSafe APIs and can incur cost. Traces from this script use the `experiment` environment.

The `docket.` score prefix separates these results from Conclave in the shared Langfuse project.

Category accuracy checks the primary filing category. Action accuracy checks whether Docket correctly distinguishes requests for a response, payment, or signature from informational documents. The fixtures use fake names. Add a reviewed fixture and label to `testdata/cases.tsv` when a new failure mode is found. Leave the action label blank for cases whose expected action has not been reviewed.

The current suite does not grade transcription fidelity, urgency, or sensitivity because those need reviewed references. Do not treat category confidence as accuracy.
