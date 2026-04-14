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

; --- Implementation of d.Div(o) ---
(define-fun dm () (_ BitVec 64) (bvashr d (_ bv63 64)))
(define-fun ud_64 () (_ BitVec 64) (bvsub (bvxor d dm) dm))
(define-fun ud () (_ BitVec 128) (zext64 ud_64))

(define-fun om () (_ BitVec 64) (bvashr o (_ bv63 64)))
(define-fun uo_64 () (_ BitVec 64) (bvsub (bvxor o om) om))
(define-fun uo () (_ BitVec 128) (zext64 uo_64))

(define-fun dividend128 () (_ BitVec 128) (bvmul ud scale_128))
(define-fun quo () (_ BitVec 128) (bvudiv dividend128 uo))
(define-fun rem () (_ BitVec 128) (bvurem dividend128 uo))

; if (rem<<1)+(quo&1) > uo { quo++ }
; (Banker's Rounding)
(define-fun bit () (_ BitVec 128) (bvand quo one_128))
(define-fun lhs () (_ BitVec 128) (bvor (bvshl rem one_128) bit))
(define-fun rounding_condition () Bool (bvugt lhs uo))

(define-fun quo_adjusted () (_ BitVec 128) 
    (ite rounding_condition (bvadd quo one_128) quo))

; --- Specification Check ---
(define-fun rem_2 () (_ BitVec 128) (bvmul rem two_128))
(define-fun is_half () Bool (= rem_2 uo))
(define-fun is_greater () Bool (bvugt rem_2 uo))
(define-fun quo_is_odd () Bool (= (bvand quo one_128) one_128))

(define-fun spec_quo_adj () (_ BitVec 128)
    (ite is_greater (bvadd quo one_128)
        (ite (and is_half quo_is_odd) (bvadd quo one_128)
            quo)))

(assert (not (= quo_adjusted spec_quo_adj)))

; Constraints: avoid division by zero (bits.Div64 panics in Go)
(assert (not (= o (_ bv0 64))))

(check-sat)
