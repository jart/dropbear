package schwab

import "fmt"

type Rule80ACode byte

const (
	Rule80ACodeA Rule80ACode = 'A' // agency single order
	Rule80ACodeB Rule80ACode = 'B' // short exempt transaction (refer to a type)
	Rule80ACodeC Rule80ACode = 'C' // program Order, non-index arb, for member firm/org
	Rule80ACodeD Rule80ACode = 'D' // program Order, index arb, for member firm/org
	Rule80ACodeE Rule80ACode = 'E' // short exempt transaction for principal (was incorrectly identified in the fix spec as "registered equity market maker trades")
	Rule80ACodeF Rule80ACode = 'F' // short exempt transaction (refer to w type)
	Rule80ACodeH Rule80ACode = 'H' // short exempt transaction (refer to i type)
	Rule80ACodeI Rule80ACode = 'I' // individual Investor, single order
	Rule80ACodeJ Rule80ACode = 'J' // program Order, index arb, for individual customer
	Rule80ACodeK Rule80ACode = 'K' // program Order, non-index arb, for individual customer
	Rule80ACodeL Rule80ACode = 'L' // short exempt transaction for member competing market-maker affiliated with the firm clearing the trade (refer to P and O types)
	Rule80ACodeM Rule80ACode = 'M' // program Order, index arb, for other member
	Rule80ACodeN Rule80ACode = 'N' // program Order, non-index arb, for other member
	Rule80ACodeO Rule80ACode = 'O' // proprietary transactions for competing market-maker that is affiliated with the clearing member (was incorrectly identified in the fix spec as "competing dealer trades")
	Rule80ACodeP Rule80ACode = 'P' // principal
	Rule80ACodeR Rule80ACode = 'R' // transactions for the account of a non-member competing market maker (was incorrectly identified in the fix spec as "competing dealer trades")
	Rule80ACodeS Rule80ACode = 'S' // specialist trades
	Rule80ACodeT Rule80ACode = 'T' // competing dealer trades
	Rule80ACodeU Rule80ACode = 'U' // program Order, index arb, for other agency
	Rule80ACodeW Rule80ACode = 'W' // all other orders as agent for other member
	Rule80ACodeX Rule80ACode = 'X' // short exempt transaction for member competing market-maker not affiliated with the firm clearing the trade (refer to w and t types)
	Rule80ACodeY Rule80ACode = 'Y' // program Order, non-index arb, for other agency
	Rule80ACodeZ Rule80ACode = 'Z' // short exempt transaction for non-member competing market-maker (refer to a and r types)
)

func (c Rule80ACode) String() string {
	return fmt.Sprintf("Rule80ACode%c", byte(c))
}
