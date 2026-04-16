(set-logic QF_BV)

; =====================================================================
; Verification of Decimal.Div rounding and sign (div.go)
;
; Combined with verify_abs.smt2, this constitutes a full proof of Div.
;
; Same strategy as verify_mul.smt2: quo and rem are universally
; quantified with the constraint rem < uo (guaranteed by bits.Div64).
; This avoids 128-bit division in the formula.
;
; Verifies:
;   1. Banker's rounding (div.go:33-35), faithfully modeled in 64-bit:
;      if (rem<<1)|(quo&1) > uo { quo++ }
;
;   2. Sign extraction and reapplication (div.go:20,44)
; =====================================================================

(declare-fun d () (_ BitVec 64))   ; first operand (signed)
(declare-fun o () (_ BitVec 64))   ; second operand (signed)
(declare-fun quo () (_ BitVec 64)) ; bits.Div64 quotient (unsigned)
(declare-fun rem () (_ BitVec 64)) ; bits.Div64 remainder (unsigned)

(define-fun zx ((v (_ BitVec 64))) (_ BitVec 128) (concat (_ bv0 64) v))

; =====================================================================
; SHARED: absolute value of o (for use as divisor bound)
; =====================================================================

(define-fun om () (_ BitVec 64) (bvashr o (_ bv63 64)))
(define-fun uo () (_ BitVec 64) (bvsub (bvxor o om) om))

; =====================================================================
; IMPLEMENTATION (div.go lines 20, 33-35, 44)
; =====================================================================

; div.go:20  sm := (di ^ oi) >> 63
(define-fun sm () (_ BitVec 64) (bvashr (bvxor d o) (_ bv63 64)))

; div.go:33-35  if (rem<<1)|(quo&1) > uo { quo++ }
(define-fun lhs () (_ BitVec 64)
  (bvor (bvshl rem (_ bv1 64)) (bvand quo (_ bv1 64))))
(define-fun impl_quo_r () (_ BitVec 64)
  (ite (bvugt lhs uo) (bvadd quo (_ bv1 64)) quo))

; div.go:44  return Decimal(int64((quo ^ uint64(sm)) - uint64(sm)))
(define-fun impl_result () (_ BitVec 64)
  (bvsub (bvxor impl_quo_r sm) sm))

; =====================================================================
; SPECIFICATION: textbook banker's rounding + conditional negation
; =====================================================================

; Round up if 2*rem > |o|, or if exactly half and quo is odd.
(define-fun spec_rem2  () (_ BitVec 128) (bvshl (zx rem) (_ bv1 128)))
(define-fun spec_uo128 () (_ BitVec 128) (zx uo))
(define-fun spec_round_up () Bool
  (or (bvugt spec_rem2 spec_uo128)
      (and (= spec_rem2 spec_uo128)
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

; o != 0 (division by zero panics)
(assert (not (= o (_ bv0 64))))

; rem < uo (guaranteed by bits.Div64 since divisor = uo)
(assert (bvult rem uo))

; =====================================================================
; PROOF OBLIGATION
; =====================================================================

(assert (not (= impl_result spec_result)))
(check-sat)
; Expected: unsat
