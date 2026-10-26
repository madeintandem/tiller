# Transcribe CERT-LTI-02-A (2025) into JSON Schema

## Approach

Read the 2025 form PDF field by field and emit a JSON Schema with the
computed-field bindings the calculator reads.

## Surfaces

- `app/models/form_definitions/`

## Tests

- `spec/form_definitions/cert_lti_02a_spec.rb`
