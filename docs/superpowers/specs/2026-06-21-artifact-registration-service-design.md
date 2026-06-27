# M4.5 Artifact Registration Service Design

## Goal

Add the domain service that registers active workspace/output files as
user-visible task artifacts.

## Scope

This slice builds on the `agent_artifacts` repository foundation. It does not
add HTTP APIs, task-detail frontend drawer wiring, preview/download handlers,
object-storage reads, MIME sniffing, deletion, retention cleanup, output
directory scanning, or audit events.

## Registration Contract

`RegisterArtifact` accepts a requested `space_id`, `thread_id`, `run_id`,
`file_id`, `artifact_type`, optional title, and JSON metadata. The service
loads the backing `agent_files` row server-side and requires:

- the file ID exists;
- `space_id`, `thread_id`, and `run_id` match the request;
- `status = active`;
- `file_kind` is `workspace` or `output`;
- size is positive;
- virtual path and object URI are present;
- artifact metadata is valid JSON;
- artifact type is non-empty.

The artifact row copies stable file metadata from `agent_files`: virtual path,
object URI, content type, size, owner, and run scope. The title defaults to
`original_file_name`, then `file_name`, then `artifact`.

## Preview Mode

Registration assigns a conservative initial preview mode from the stored
content type:

- `text/plain`, Markdown, CSV, TSV, and JSON become `text`;
- common raster images become `image`;
- PDF becomes `pdf`;
- HTML, XHTML, SVG, unknown binary, and all other content types become
  `download`.

This is only an initial registry hint. Future preview/download APIs must still
perform server-side MIME sniffing and force active content to attachment
downloads.

## Security Boundary

The client does not get to choose the object URI, virtual path, file owner, or
previewable active content policy. Those values come from the server-side file
record and Coze-owned classification.

Errors remain content-free and must not expose object URIs, stored file
content, prompt text, completion text, checkpoint bytes, credentials, object
storage diagnostics, or provider raw responses.

## Testing

Domain tests cover successful registration, title defaults, active-content
download fallback, preview mode classification, and rejection of cross-scope,
deleted, upload-kind, and invalid-metadata inputs.
