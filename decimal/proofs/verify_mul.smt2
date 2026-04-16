(set-logic QF_BV)

; =====================================================================
; Verification of Decimal.Mul rounding and sign (mul.go)
;
; Combined with verify_abs.smt2, this constitutes a full proof of Mul.
;
; The 128-bit multiply and divide (bits.Mul64/Div64) are standard
; library functions whose correctness is assumed. This proof verifies
; the two non-trivial parts the code implements by hand:
;
;   1. The banker's rounding bit trick (mul.go:28), faithfully modeled
;      in 64-bit arithmetic matching the exact Go expression:
;      quo -= uint64(int64(uint64(Scale)-(rem<<1|quo&1)) >> 63)
;
;   2. Sign extraction and reapplication (mul.go:18,40):
;      sm := (di ^ oi) >> 63
;      result := int64((quo ^ uint64(sm)) - uint64(sm))
;
; quo and rem are universally quantified with the constraint rem < Scale
; (guaranteed by bits.Div64). This avoids 128-bit division in the
; formula, keeping the proof fast.
; =====================================================================

(declare-fun d () (_ BitVec 64))   ; first operand (signed)
(declare-fun o () (_ BitVec 64))   ; second operand (signed)
(declare-fun quo () (_ BitVec 64)) ; bits.Div64 quotient (unsigned)
(declare-fun rem () (_ BitVec 64)) ; bits.Div64 remainder (unsigned)

(define-fun SCALE    () (_ BitVec 64)  (_ bv1000000 64))
(define-fun SCALE128 () (_ BitVec 128) (_ bv1000000 128))
(define-fun zx ((v (_ BitVec 64))) (_ BitVec 128) (concat (_ bv0 64) v))

; =====================================================================
; IMPLEMENTATION (mul.go lines 18, 28, 40)
; =====================================================================

; mul.go:18  sm := (di ^ oi) >> 63
(define-fun sm () (_ BitVec 64) (bvashr (bvxor d o) (_ bv63 64)))

; mul.go:28  quo -= uint64(int64(uint64(Scale)-(rem<<1|quo&1)) >> 63)
(define-fun lhs  () (_ BitVec 64)
  (bvor (bvshl rem (_ bv1 64)) (bvand quo (_ bv1 64))))
(define-fun diff () (_ BitVec 64) (bvsub SCALE lhs))
(define-fun adj  () (_ BitVec 64) (bvashr diff (_ bv63 64)))
(define-fun impl_quo_r () (_ BitVec 64) (bvsub quo adj))

; mul.go:40  return Decimal(int64((quo ^ uint64(sm)) - uint64(sm)))
(define-fun impl_result () (_ BitVec 64)
  (bvsub (bvxor impl_quo_r sm) sm))

; =====================================================================
; SPECIFICATION: textbook banker's rounding + conditional negation
; =====================================================================

; Round up if 2*rem > Scale, or if exactly half and quo is odd.
; (rem widened to 128-bit so 2*rem is obviously overflow-free)
(define-fun spec_rem2 () (_ BitVec 128) (bvshl (zx rem) (_ bv1 128)))
(define-fun spec_round_up () Bool
  (or (bvugt spec_rem2 SCALE128)
      (and (= spec_rem2 SCALE128)
           (= (bvand quo (_ bv1 64)) (_ bv1 64)))))
(define-fun spec_quo_r () (_ BitVec 64)
  (ite spec_round_up (bvadd quo (_ bv1 64)) quo))

; Negate iff exactly one of d, o is negative.
(define-fun spec_neg () Bool
  (xor (bvslt d (_ bv0 64)) (bvslt o (_ bv0 64))))
(define-fun spec_result () (_ BitVec 64)
  (ite spec_neg (bvneg spec_quo_r) spec_quo_r))

; =====================================================================
; CONSTRAINTS
; =====================================================================

; rem < Scale (guaranteed by bits.Div64 since divisor = Scale)
(assert (bvult rem SCALE))

; =====================================================================
; PROOF OBLIGATION
; =====================================================================

(assert (not (= impl_result spec_result)))
(check-sat)
; Expected: unsat
