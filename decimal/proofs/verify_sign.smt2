(set-logic QF_BV)

; Verify the branchless sign logic used in mul.go:18,40 and div.go:20,44.
;
; Sign extraction:
;   sm := (d ^ o) >> 63     (0 if same sign, -1 if different)
;
; Sign application:
;   result := (r ^ uint64(sm)) - uint64(sm)
;   When sm =  0: result = r        (same sign, keep positive)
;   When sm = -1: result = ~r+1 = -r (different signs, negate)
;
; Specification:
;   If exactly one of d, o is negative (signed): result = -r
;   Otherwise: result = r

(declare-fun d () (_ BitVec 64))
(declare-fun o () (_ BitVec 64))
(declare-fun r () (_ BitVec 64))

(define-fun sm   () (_ BitVec 64) (bvashr (bvxor d o) (_ bv63 64)))
(define-fun impl () (_ BitVec 64) (bvsub (bvxor r sm) sm))
(define-fun neg  () Bool (xor (bvslt d (_ bv0 64)) (bvslt o (_ bv0 64))))
(define-fun spec () (_ BitVec 64) (ite neg (bvneg r) r))

(assert (not (= impl spec)))
(check-sat)
; Expected: unsat
