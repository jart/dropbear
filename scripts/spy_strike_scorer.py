#!/usr/bin/env python3
"""Derive and validate scoring functions for SPX 0DTE strike selection.

Fits the empirical distribution of SPY daily moves to parametric models,
validates them, and outputs simple scoring formulas for calls and puts.

Usage:
    python3 scripts/spy_strike_scorer.py
"""

import os
import sqlite3
import math
from itertools import product

ANALYSIS_DB = os.path.expanduser("~/.spy_daily_moves.sqlite3")


def phi(x):
    """Standard normal PDF."""
    return math.exp(-x * x / 2) / math.sqrt(2 * math.pi)


def Phi(x):
    """Standard normal CDF."""
    return 0.5 * math.erfc(-x / math.sqrt(2))


def load_moves():
    conn = sqlite3.connect(ANALYSIS_DB)
    rows = conn.execute("SELECT pct_move FROM spy_daily_moves ORDER BY date").fetchall()
    conn.close()
    return [r[0] for r in rows]


def fit_moments(moves):
    n = len(moves)
    mu = sum(moves) / n
    var = sum((m - mu) ** 2 for m in moves) / n
    sigma = var ** 0.5
    skew = sum((m - mu) ** 3 for m in moves) / n / sigma ** 3
    kurt = sum((m - mu) ** 4 for m in moves) / n / sigma ** 4
    return mu, sigma, skew, kurt


# --- Normal model ---

def call_ev_normal(x, mu, sigma):
    """E[max(0, M - x)] where M ~ N(mu, sigma)."""
    z = (x - mu) / sigma
    return (mu - x) * Phi(-z) + sigma * phi(z)


def put_ev_normal(x, mu, sigma):
    """E[max(0, -x - M)] where M ~ N(mu, sigma)."""
    z = (x + mu) / sigma
    return (-mu - x) * Phi(-z) + sigma * phi(z)


# --- Mixture of two normals ---
# M ~ w * N(mu, s1) + (1-w) * N(mu, s2)
# EV is just the weighted sum of the component EVs.

def call_ev_mix(x, mu, w, s1, s2):
    return w * call_ev_normal(x, mu, s1) + (1 - w) * call_ev_normal(x, mu, s2)


def put_ev_mix(x, mu, w, s1, s2):
    return w * put_ev_normal(x, mu, s1) + (1 - w) * put_ev_normal(x, mu, s2)


def call_ev_empirical(x, moves):
    payoffs = [max(0, m - x) for m in moves]
    return sum(payoffs) / len(payoffs)


def put_ev_empirical(x, moves):
    payoffs = [max(0, -m - x) for m in moves]
    return sum(payoffs) / len(payoffs)


def fit_mixture(moves, mu):
    """Fit a two-component normal mixture by grid search minimizing EV error."""
    test_otms = [0.0, 0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 2.00, 2.50, 3.00]
    emp_call = {x: call_ev_empirical(x, moves) for x in test_otms}
    emp_put = {x: put_ev_empirical(x, moves) for x in test_otms}

    best = None
    best_err = float('inf')

    # Grid search: w in [0.5, 0.95], s1 in [0.3, 1.0], s2 in [1.0, 5.0]
    for w100 in range(50, 96, 2):
        w = w100 / 100
        for s1_10 in range(3, 11):
            s1 = s1_10 / 10
            for s2_10 in range(10, 60, 2):
                s2 = s2_10 / 10
                # Constraint: overall variance should match empirical
                # Var = w*s1^2 + (1-w)*s2^2 (when means are the same)
                err = 0
                for x in test_otms:
                    mc = call_ev_mix(x, mu, w, s1, s2)
                    mp = put_ev_mix(x, mu, w, s1, s2)
                    # Weight errors by 1/max(emp,0.001) to get relative error
                    ec = emp_call[x]
                    ep = emp_put[x]
                    if ec > 0.0001:
                        err += ((mc - ec) / ec) ** 2
                    if ep > 0.0001:
                        err += ((mp - ep) / ep) ** 2
                if err < best_err:
                    best_err = err
                    best = (w, s1, s2)

    # Refine around best
    w0, s10, s20 = best
    for dw in range(-5, 6):
        w = w0 + dw / 200
        if w <= 0 or w >= 1:
            continue
        for ds1 in range(-5, 6):
            s1 = s10 + ds1 / 50
            if s1 <= 0:
                continue
            for ds2 in range(-10, 11):
                s2 = s20 + ds2 / 25
                if s2 <= 0:
                    continue
                err = 0
                for x in test_otms:
                    mc = call_ev_mix(x, mu, w, s1, s2)
                    mp = put_ev_mix(x, mu, w, s1, s2)
                    ec = emp_call[x]
                    ep = emp_put[x]
                    if ec > 0.0001:
                        err += ((mc - ec) / ec) ** 2
                    if ep > 0.0001:
                        err += ((mp - ep) / ep) ** 2
                if err < best_err:
                    best_err = err
                    best = (w, s1, s2)

    return best, best_err


def validate(moves, mu, w, s1, s2, sigma):
    """Compare mixture model vs normal vs empirical."""
    n = len(moves)
    print("=" * 90)
    print("MODEL VALIDATION: Mixture vs Normal vs Empirical")
    print("=" * 90)

    print(f"\nCALLS: E[max(0, move - x)]")
    print(f"{'OTM%':>6s}  {'Empirical':>10s}  {'Mixture':>10s}  {'Mix Err':>8s}  {'Normal':>10s}  {'Norm Err':>9s}")
    print("-" * 70)
    for x in [0.0, 0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 2.00, 2.50, 3.00]:
        emp = call_ev_empirical(x, moves)
        mix = call_ev_mix(x, mu, w, s1, s2)
        norm = call_ev_normal(x, mu, sigma)
        mix_err = (mix / emp - 1) * 100 if emp > 0 else 0
        norm_err = (norm / emp - 1) * 100 if emp > 0 else 0
        print(f"  {x:.2f}   {emp:.5f}     {mix:.5f}   {mix_err:+5.0f}%     {norm:.5f}    {norm_err:+5.0f}%")

    print(f"\nPUTS: E[max(0, -move - x)]")
    print(f"{'OTM%':>6s}  {'Empirical':>10s}  {'Mixture':>10s}  {'Mix Err':>8s}  {'Normal':>10s}  {'Norm Err':>9s}")
    print("-" * 70)
    for x in [0.0, 0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 2.00, 2.50, 3.00]:
        emp = put_ev_empirical(x, moves)
        mix = put_ev_mix(x, mu, w, s1, s2)
        norm = put_ev_normal(x, mu, sigma)
        mix_err = (mix / emp - 1) * 100 if emp > 0 else 0
        norm_err = (norm / emp - 1) * 100 if emp > 0 else 0
        print(f"  {x:.2f}   {emp:.5f}     {mix:.5f}   {mix_err:+5.0f}%     {norm:.5f}    {norm_err:+5.0f}%")

    # Also validate P(ITM)
    print(f"\nP(ITM) VALIDATION:")
    print(f"{'OTM%':>6s}  {'Emp Call':>10s}  {'Mix Call':>10s}  {'Emp Put':>10s}  {'Mix Put':>10s}")
    print("-" * 55)
    for x in [0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 2.00, 2.50, 3.00]:
        emp_c = sum(1 for m in moves if m >= x) / n * 100
        emp_p = sum(1 for m in moves if m <= -x) / n * 100
        # Mixture CDF
        z1c = (x - mu) / s1
        z2c = (x - mu) / s2
        mix_c = (w * Phi(-z1c) + (1 - w) * Phi(-z2c)) * 100
        z1p = (-x - mu) / s1
        z2p = (-x - mu) / s2
        mix_p = (w * Phi(z1p) + (1 - w) * Phi(z2p)) * 100
        print(f"  {x:.2f}   {emp_c:6.1f}%     {mix_c:6.1f}%     {emp_p:6.1f}%     {mix_p:6.1f}%")


def print_formulas(mu, w, s1, s2):
    print("\n" + "=" * 90)
    print("FINAL SCORING FORMULAS (Mixture of Normals)")
    print("=" * 90)

    mix_var = w * s1**2 + (1 - w) * s2**2
    mix_sigma = mix_var ** 0.5

    print(f"""
Model: {w*100:.0f}% of days are "calm" (σ₁={s1:.3f}%), {(1-w)*100:.0f}% are "volatile" (σ₂={s2:.3f}%)
Effective σ = √(w·σ₁² + (1-w)·σ₂²) = {mix_sigma:.3f}%

Parameters:
  μ  = {mu:.4f}    (mean daily move %)
  w  = {w:.4f}    (fraction of calm days)
  s1 = {s1:.4f}    (calm day volatility %)
  s2 = {s2:.4f}    (volatile day volatility %)

Inputs to the scoring functions:
  strike = option strike price (e.g. 6050)
  cost   = quoted option price (e.g. 0.10 means $10/contract)
  spx    = current SPX index price (e.g. 6000)

Formula:
  Each component's EV uses the standard partial expectation:
    ev_i(x, σ) = (μ - x)·Φ(-z) + σ·φ(z),  where z = (x - μ)/σ  [calls]
    ev_i(x, σ) = (-μ - x)·Φ(-z) + σ·φ(z),  where z = (x + μ)/σ  [puts]

  Total EV = w·ev(x, s1) + (1-w)·ev(x, s2)
  Score = EV · spx / (cost · 100)

Interpretation:
  score > 1  →  option is CHEAP relative to historical moves
  score < 1  →  option is EXPENSIVE
  Pick the highest score each day.
""")

    print("--- Go implementation ---")
    print(f"""
import "math"

const (
    mu = {mu:.4f}
    w  = {w:.4f}
    s1 = {s1:.4f}
    s2 = {s2:.4f}
)

func phi(x float64) float64 {{
    return math.Exp(-x*x/2) / math.Sqrt(2*math.Pi)
}}

func cdf(x float64) float64 {{
    return 0.5 * math.Erfc(-x/math.Sqrt2)
}}

func callEV(x, sigma float64) float64 {{
    z := (x - mu) / sigma
    return (mu-x)*cdf(-z) + sigma*phi(z)
}}

func putEV(x, sigma float64) float64 {{
    z := (x + mu) / sigma
    return (-mu-x)*cdf(-z) + sigma*phi(z)
}}

// CallScore scores an SPX 0DTE call option.
// strike: option strike price (e.g. 6050)
// cost: quoted option price (e.g. 0.10 means $10/contract)
// spx: current SPX price
// Returns expected profit per dollar spent. >1 means cheap.
func CallScore(strike, cost, spx float64) float64 {{
    otmPct := (strike - spx) / spx * 100
    ev := w*callEV(otmPct, s1) + (1-w)*callEV(otmPct, s2)
    return ev * spx / (cost * 100)
}}

// PutScore scores an SPX 0DTE put option.
func PutScore(strike, cost, spx float64) float64 {{
    otmPct := (spx - strike) / spx * 100
    ev := w*putEV(otmPct, s1) + (1-w)*putEV(otmPct, s2)
    return ev * spx / (cost * 100)
}}
""")

    print("--- Python implementation ---")
    print(f"""
import math

MU = {mu:.4f}
W  = {w:.4f}
S1 = {s1:.4f}
S2 = {s2:.4f}

def _phi(x):
    return math.exp(-x*x/2) / math.sqrt(2*math.pi)

def _cdf(x):
    return 0.5 * math.erfc(-x / math.sqrt(2))

def _call_ev(x, s):
    z = (x - MU) / s
    return (MU - x) * _cdf(-z) + s * _phi(z)

def _put_ev(x, s):
    z = (x + MU) / s
    return (-MU - x) * _cdf(-z) + s * _phi(z)

def call_score(strike, cost, spx):
    otm_pct = (strike - spx) / spx * 100
    ev = W * _call_ev(otm_pct, S1) + (1 - W) * _call_ev(otm_pct, S2)
    return ev * spx / (cost * 100)

def put_score(strike, cost, spx):
    otm_pct = (spx - strike) / spx * 100
    ev = W * _put_ev(otm_pct, S1) + (1 - W) * _put_ev(otm_pct, S2)
    return ev * spx / (cost * 100)
""")

    print("--- C# implementation (QuantConnect) ---")
    print(f"""
using System;

public static class LotteryScorer
{{
    const double Mu = {mu:.4f};
    const double W  = {w:.4f};
    const double S1 = {s1:.4f};
    const double S2 = {s2:.4f};

    static double Phi(double x)
    {{
        return Math.Exp(-x * x / 2) / Math.Sqrt(2 * Math.PI);
    }}

    static double Cdf(double x)
    {{
        return 0.5 * Erfc(-x / Math.Sqrt(2));
    }}

    static double Erfc(double x)
    {{
        // Abramowitz and Stegun approximation 7.1.26
        double t = 1.0 / (1.0 + 0.3275911 * Math.Abs(x));
        double y = t * (0.254829592 + t * (-0.284496736 + t * (1.421413741
                 + t * (-1.453152027 + t * 1.061405429))));
        double v = y * Math.Exp(-x * x);
        return x >= 0 ? v : 2.0 - v;
    }}

    static double CallEV(double x, double sigma)
    {{
        double z = (x - Mu) / sigma;
        return (Mu - x) * Cdf(-z) + sigma * Phi(z);
    }}

    static double PutEV(double x, double sigma)
    {{
        double z = (x + Mu) / sigma;
        return (-Mu - x) * Cdf(-z) + sigma * Phi(z);
    }}

    /// <summary>
    /// Score an SPX 0DTE call option.
    /// Returns expected profit per dollar spent. >1 means cheap.
    /// </summary>
    public static double CallScore(double strike, double cost, double spx)
    {{
        double otmPct = (strike - spx) / spx * 100;
        double ev = W * CallEV(otmPct, S1) + (1 - W) * CallEV(otmPct, S2);
        return ev * spx / (cost * 100);
    }}

    /// <summary>
    /// Score an SPX 0DTE put option.
    /// Returns expected profit per dollar spent. >1 means cheap.
    /// </summary>
    public static double PutScore(double strike, double cost, double spx)
    {{
        double otmPct = (spx - strike) / spx * 100;
        double ev = W * PutEV(otmPct, S1) + (1 - W) * PutEV(otmPct, S2);
        return ev * spx / (cost * 100);
    }}
}}
""")


def print_score_table(mu, w, s1, s2, spx):
    """Show example scores."""
    print(f"\n{'='*80}")
    print(f"EXAMPLE SCORES (SPX = {spx:.0f})")
    print(f"Score > 1.0 means option looks cheap vs historical distribution")
    print(f"{'='*80}")

    costs = [0.05, 0.10, 0.25, 0.50, 1.00, 2.00, 5.00]
    otms = [0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 2.00, 2.50, 3.00]

    def cs(x, c):
        ev = w * call_ev_normal(x, mu, s1) + (1 - w) * call_ev_normal(x, mu, s2)
        return ev * spx / (c * 100)

    def ps(x, c):
        ev = w * put_ev_normal(x, mu, s1) + (1 - w) * put_ev_normal(x, mu, s2)
        return ev * spx / (c * 100)

    print(f"\nCALL SCORES:")
    header = f"{'OTM%':>6s}" + "".join(f"  ${c:<5.2f}" for c in costs)
    print(header)
    print("-" * (6 + 8 * len(costs)))
    for x in otms:
        row = f"  {x:.2f}"
        for c in costs:
            s = cs(x, c)
            row += f"  {s:6.2f}"
        print(row)

    print(f"\nPUT SCORES:")
    print(header)
    print("-" * (6 + 8 * len(costs)))
    for x in otms:
        row = f"  {x:.2f}"
        for c in costs:
            s = ps(x, c)
            row += f"  {s:6.2f}"
        print(row)

    # Breakeven costs
    print(f"\nBREAKEVEN OPTION PRICES (score = 1.0):")
    print(f"At these prices, expected payoff exactly equals cost.")
    print(f"{'OTM%':>6s}  {'Call $':>8s}  {'Put $':>8s}")
    print("-" * 28)
    for x in otms:
        cev = w * call_ev_normal(x, mu, s1) + (1 - w) * call_ev_normal(x, mu, s2)
        pev = w * put_ev_normal(x, mu, s1) + (1 - w) * put_ev_normal(x, mu, s2)
        call_be = cev * spx / 100  # breakeven quoted price
        put_be = pev * spx / 100
        print(f"  {x:.2f}   ${call_be:6.2f}   ${put_be:6.2f}")


def main():
    moves = load_moves()
    n = len(moves)
    print(f"Loaded {n} daily moves\n")

    mu, sigma, skew, kurt = fit_moments(moves)
    print(f"Empirical moments:")
    print(f"  Mean:     {mu:+.4f}%")
    print(f"  Std Dev:  {sigma:.4f}%")
    print(f"  Skewness: {skew:+.3f}  (normal=0)")
    print(f"  Kurtosis: {kurt:.1f}  (normal=3)")
    print(f"\n  → Kurtosis {kurt:.0f}x normal! A single normal is a terrible fit.")
    print(f"  → Using mixture of two normals: calm days + volatile days.\n")

    print("Fitting mixture model (grid search)...")
    (w, s1, s2), err = fit_mixture(moves, mu)
    mix_var = w * s1 ** 2 + (1 - w) * s2 ** 2
    print(f"  Best fit: w={w:.3f}, σ₁={s1:.3f}%, σ₂={s2:.3f}%")
    print(f"  Effective σ = {mix_var**0.5:.3f}%")
    print(f"  Relative MSE: {err:.4f}")

    validate(moves, mu, w, s1, s2, sigma)
    print_formulas(mu, w, s1, s2)
    print_score_table(mu, w, s1, s2, 6000)


if __name__ == "__main__":
    main()
