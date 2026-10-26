# Certification form renders blank after a reload mid-entry

## Reproduction

Open the certification form, fill two fields, reload.

## Expected

The two fields keep their values.

## Actual

The form renders blank.

## Fixed when

A reload mid-entry restores the entered values, covered by a system test.
