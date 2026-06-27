# M4.10 Artifact MIME Sniffing Plan

## Checklist

- [x] Add red application tests for unsafe sniffed bytes overriding safe
      metadata.
- [x] Add red application tests for unsafe metadata overriding safe sniffed
      bytes.
- [x] Add a red handler test for effective response content type.
- [x] Implement read-time MIME sniffing in `ReadArtifactContent`.
- [x] Update conservative disposition to require both metadata and sniffed
      types to be preview-safe.
- [x] Keep audit payloads content-free while recording the effective content
      type and attachment decision.
- [x] Update AGENTS.md and the DeerFlow parity roadmap.
- [x] Run targeted backend tests and diff checks.

## Notes

- Use the Go standard library for byte sniffing.
- Do not add new dependencies, migrations, scanner workers, or preview
  conversion services in this slice.
- Keep historical empty artifacts compatible through the existing
  `application/octet-stream` fallback.
