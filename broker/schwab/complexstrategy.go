package schwab

import "fmt"

type ComplexStrategy uint8

const (
	ComplexStrategyNone                   ComplexStrategy = iota // single leg, no complex strategy
	ComplexStrategyCovered                                       // covered call/put
	ComplexStrategyVertical                                      // vertical spread
	ComplexStrategyBackRatio                                     // back ratio spread
	ComplexStrategyCalendar                                      // calendar spread
	ComplexStrategyDiagonal                                      // diagonal spread
	ComplexStrategyStraddle                                      // straddle
	ComplexStrategyStrangle                                      // strangle
	ComplexStrategyCollarSynthetic                               // synthetic collar
	ComplexStrategyButterfly                                     // butterfly spread
	ComplexStrategyCondor                                        // condor spread
	ComplexStrategyIronCondor                                    // iron condor
	ComplexStrategyVerticalRoll                                  // vertical roll
	ComplexStrategyCollarWithStock                               // collar with stock
	ComplexStrategyDoubleDiagonal                                // double diagonal
	ComplexStrategyUnbalancedButterfly                           // unbalanced butterfly
	ComplexStrategyUnbalancedCondor                              // unbalanced condor
	ComplexStrategyUnbalancedIronCondor                          // unbalanced iron condor
	ComplexStrategyUnbalancedVerticalRoll                        // unbalanced vertical roll
	ComplexStrategyMutualFundSwap                                // mutual fund swap
	ComplexStrategyCustom                                        // custom strategy
)

func ParseComplexStrategy(s string) (ComplexStrategy, error) {
	switch s {
	case "NONE":
		return ComplexStrategyNone, nil
	case "COVERED":
		return ComplexStrategyCovered, nil
	case "VERTICAL":
		return ComplexStrategyVertical, nil
	case "BACK_RATIO":
		return ComplexStrategyBackRatio, nil
	case "CALENDAR":
		return ComplexStrategyCalendar, nil
	case "DIAGONAL":
		return ComplexStrategyDiagonal, nil
	case "STRADDLE":
		return ComplexStrategyStraddle, nil
	case "STRANGLE":
		return ComplexStrategyStrangle, nil
	case "COLLAR_SYNTHETIC":
		return ComplexStrategyCollarSynthetic, nil
	case "BUTTERFLY":
		return ComplexStrategyButterfly, nil
	case "CONDOR":
		return ComplexStrategyCondor, nil
	case "IRON_CONDOR":
		return ComplexStrategyIronCondor, nil
	case "VERTICAL_ROLL":
		return ComplexStrategyVerticalRoll, nil
	case "COLLAR_WITH_STOCK":
		return ComplexStrategyCollarWithStock, nil
	case "DOUBLE_DIAGONAL":
		return ComplexStrategyDoubleDiagonal, nil
	case "UNBALANCED_BUTTERFLY":
		return ComplexStrategyUnbalancedButterfly, nil
	case "UNBALANCED_CONDOR":
		return ComplexStrategyUnbalancedCondor, nil
	case "UNBALANCED_IRON_CONDOR":
		return ComplexStrategyUnbalancedIronCondor, nil
	case "UNBALANCED_VERTICAL_ROLL":
		return ComplexStrategyUnbalancedVerticalRoll, nil
	case "MUTUAL_FUND_SWAP":
		return ComplexStrategyMutualFundSwap, nil
	case "CUSTOM":
		return ComplexStrategyCustom, nil
	default:
		return 0, fmt.Errorf("unknown complex strategy: %s", s)
	}
}

func (c ComplexStrategy) String() string {
	switch c {
	case ComplexStrategyNone:
		return "NONE"
	case ComplexStrategyCovered:
		return "COVERED"
	case ComplexStrategyVertical:
		return "VERTICAL"
	case ComplexStrategyBackRatio:
		return "BACK_RATIO"
	case ComplexStrategyCalendar:
		return "CALENDAR"
	case ComplexStrategyDiagonal:
		return "DIAGONAL"
	case ComplexStrategyStraddle:
		return "STRADDLE"
	case ComplexStrategyStrangle:
		return "STRANGLE"
	case ComplexStrategyCollarSynthetic:
		return "COLLAR_SYNTHETIC"
	case ComplexStrategyButterfly:
		return "BUTTERFLY"
	case ComplexStrategyCondor:
		return "CONDOR"
	case ComplexStrategyIronCondor:
		return "IRON_CONDOR"
	case ComplexStrategyVerticalRoll:
		return "VERTICAL_ROLL"
	case ComplexStrategyCollarWithStock:
		return "COLLAR_WITH_STOCK"
	case ComplexStrategyDoubleDiagonal:
		return "DOUBLE_DIAGONAL"
	case ComplexStrategyUnbalancedButterfly:
		return "UNBALANCED_BUTTERFLY"
	case ComplexStrategyUnbalancedCondor:
		return "UNBALANCED_CONDOR"
	case ComplexStrategyUnbalancedIronCondor:
		return "UNBALANCED_IRON_CONDOR"
	case ComplexStrategyUnbalancedVerticalRoll:
		return "UNBALANCED_VERTICAL_ROLL"
	case ComplexStrategyMutualFundSwap:
		return "MUTUAL_FUND_SWAP"
	case ComplexStrategyCustom:
		return "CUSTOM"
	default:
		panic("unknown complex strategy")
	}
}

func (c ComplexStrategy) GoString() string {
	switch c {
	case ComplexStrategyNone:
		return "ComplexStrategyNone"
	case ComplexStrategyCovered:
		return "ComplexStrategyCovered"
	case ComplexStrategyVertical:
		return "ComplexStrategyVertical"
	case ComplexStrategyBackRatio:
		return "ComplexStrategyBackRatio"
	case ComplexStrategyCalendar:
		return "ComplexStrategyCalendar"
	case ComplexStrategyDiagonal:
		return "ComplexStrategyDiagonal"
	case ComplexStrategyStraddle:
		return "ComplexStrategyStraddle"
	case ComplexStrategyStrangle:
		return "ComplexStrategyStrangle"
	case ComplexStrategyCollarSynthetic:
		return "ComplexStrategyCollarSynthetic"
	case ComplexStrategyButterfly:
		return "ComplexStrategyButterfly"
	case ComplexStrategyCondor:
		return "ComplexStrategyCondor"
	case ComplexStrategyIronCondor:
		return "ComplexStrategyIronCondor"
	case ComplexStrategyVerticalRoll:
		return "ComplexStrategyVerticalRoll"
	case ComplexStrategyCollarWithStock:
		return "ComplexStrategyCollarWithStock"
	case ComplexStrategyDoubleDiagonal:
		return "ComplexStrategyDoubleDiagonal"
	case ComplexStrategyUnbalancedButterfly:
		return "ComplexStrategyUnbalancedButterfly"
	case ComplexStrategyUnbalancedCondor:
		return "ComplexStrategyUnbalancedCondor"
	case ComplexStrategyUnbalancedIronCondor:
		return "ComplexStrategyUnbalancedIronCondor"
	case ComplexStrategyUnbalancedVerticalRoll:
		return "ComplexStrategyUnbalancedVerticalRoll"
	case ComplexStrategyMutualFundSwap:
		return "ComplexStrategyMutualFundSwap"
	case ComplexStrategyCustom:
		return "ComplexStrategyCustom"
	default:
		panic("unknown complex strategy")
	}
}

func (c ComplexStrategy) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

func (c *ComplexStrategy) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid complex strategy: %s", data)
	}
	v, err := ParseComplexStrategy(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*c = v
	return nil
}
