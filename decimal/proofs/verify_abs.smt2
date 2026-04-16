(set-logic QF_BV)

; Verify the branchless absolute value used in mul.go:14-15 and div.go:16-17.
;
;   m := x >> 63          (arithmetic shift: 0 or -1)
;   abs := (x ^ m) - m    (absolute value as uint64)
;
; Specification: abs = (x < 0) ? -x : x
;
; Both expressions produce identical bit patterns for ALL int64 values,
; including x = MinInt64 where |x| = 2^63 (wraps in int64, but is
; correct when zero-extended to uint64/uint128 for subsequent arithmetic).

(declare-fun x () (_ BitVec 64))

(define-fun m    () (_ BitVec 64) (bvashr x (_ bv63 64)))
(define-fun impl () (_ BitVec 64) (bvsub (bvxor x m) m))
(define-fun spec () (_ BitVec 64) (ite (bvslt x (_ bv0 64)) (bvneg x) x))

(assert (not (= impl spec)))
(check-sat)
; Expected: unsat
