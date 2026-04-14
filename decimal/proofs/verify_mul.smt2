(set-logic QF_BV)

; --- 64-bit Inputs ---
(declare-fun d () (_ BitVec 64))
(declare-fun o () (_ BitVec 64))

; --- Helper for 128-bit Zero Extension ---
(define-fun zext64 ( (x (_ BitVec 64)) ) (_ BitVec 128)
    (concat (_ bv0 64) x)
)

; --- Constants (128-bit) ---
(define-fun one_128 () (_ BitVec 128) (_ bv1 128))
(define-fun two_128 () (_ BitVec 128) (_ bv2 128))
(define-fun scale_128 () (_ BitVec 128) (_ bv1000000 128))

; --- Implementation of d.Mul(o) ---
(define-fun dm () (_ BitVec 64) (bvashr d (_ bv63 64)))
(define-fun ud_64 () (_ BitVec 64) (bvsub (bvxor d dm) dm))
(define-fun ud () (_ BitVec 128) (zext64 ud_64))

(define-fun om () (_ BitVec 64) (bvashr o (_ bv63 64)))
(define-fun uo_64 () (_ BitVec 64) (bvsub (bvxor o om) om))
(define-fun uo () (_ BitVec 128) (zext64 uo_64))

(define-fun prod128 () (_ BitVec 128) (bvmul ud uo))
(define-fun quo_128 () (_ BitVec 128) (bvudiv prod128 scale_128))
(define-fun rem_128 () (_ BitVec 128) (bvurem prod128 scale_128))

; (Banker's Rounding)
; quo -= uint64(int64(uint64(Scale)-(rem<<1|quo&1)) >> 63)
(define-fun bit_128 () (_ BitVec 128) (bvand quo_128 one_128))
(define-fun lhs_128 () (_ BitVec 128) (bvor (bvshl rem_128 one_128) bit_128))
(define-fun diff_128 () (_ BitVec 128) (bvsub scale_128 lhs_128))

; In Go, int64(Scale - lhs) >> 63 is -1 if Scale - lhs is negative.
; Scale - lhs is negative if Scale < lhs (unsigned).
; This is exactly what we need to check.
(define-fun rounding_condition () Bool
    (bvult scale_128 lhs_128)
)

(define-fun quo_adjusted () (_ BitVec 128) 
    (ite rounding_condition (bvadd quo_128 one_128) quo_128))

; --- Specification Check ---
(define-fun rem_2 () (_ BitVec 128) (bvmul rem_128 two_128))
(define-fun is_half () Bool (= rem_2 scale_128))
(define-fun is_greater () Bool (bvugt rem_2 scale_128))
(define-fun quo_is_odd () Bool (= (bvand quo_128 one_128) one_128))

(define-fun spec_quo_adj () (_ BitVec 128)
    (ite is_greater (bvadd quo_128 one_128)
        (ite (and is_half quo_is_odd) (bvadd quo_128 one_128)
            quo_128)))

(assert (not (= quo_adjusted spec_quo_adj)))

(check-sat)
