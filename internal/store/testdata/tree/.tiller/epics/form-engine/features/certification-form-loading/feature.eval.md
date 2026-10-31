---
node: F-79dh3q
criteria:
  - id: C1
    statement: FORM-A-100 (2025) validates against its JSON Schema
    polarity: introduce
    binding: { type: test, ref: "spec/form_definitions/form_a_100_spec.rb" }
    required: true
  - id: C2
    statement: Parent + child render to a single PDF with continuous page numbers
    polarity: introduce
    binding: { type: command, run: "bundle exec rspec spec/pdf/cert_golden_spec.rb" }
    required: true
  - id: C3
    statement: A reviewer confirms the rendered PDF matches the source form layout
    polarity: introduce
    binding: { type: manual }
    required: true
---

Reviewer notes and the human elaboration of each criterion go here.
