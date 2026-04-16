#!/bin/bash
# run all z3 proofs
cd "$(dirname "$0")"

if ! Z3=$(command -v z3); then
  echo please install z3 >&2
  exit 1
fi

pass=0
fail=0
for f in verify_*.smt2; do
  printf "%-30s " "$f"
  result=$("$Z3" -T:300 "$f" 2>&1)
  echo "$result"
  if [ "$result" = "unsat" ]; then
    ((pass++))
  else
    ((fail++))
  fi
done

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
