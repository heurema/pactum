# Corrective Reviewer Prompt

You are the implementation reviewer.

Your previous response did not include a valid `pactum.reviewer_findings.v1alpha1` JSON block.
You MUST emit exactly one fenced JSON block. If you have no findings, emit `"findings": []`.

```json
{
  "schema": "pactum.reviewer_findings.v1alpha1",
  "findings": []
}
```
