package cubby

import (
	"log"
	"os"

	"golang.org/x/term"
)

func rekt() {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		log.Printf("")
		log.Printf("     .... NO! ...                  ... MNO! ...")
		log.Printf("   ..... MNO!! ...................... MNNOO! ...")
		log.Printf(" ..... MMNO! ......................... MNNOO!! .")
		log.Printf("..... MNOONNOO!   MMMMMMMMMMPPPOII!   MNNO!!!! .")
		log.Printf(" ... !O! NNO! MMMMMMMMMMMMMPPPOOOII!! NO! ....")
		log.Printf("    ...... ! MMMMMMMMMMMMMPPPPOOOOIII! ! ...")
		log.Printf("   ........ MMMMMMMMMMMMPPPPPOOOOOOII!! .....")
		log.Printf("   ........ MMMMMOOOOOOPPPPPPPPOOOOMII! ...")
		log.Printf("    ....... MMMMM..    OPPMMP    .,OMI! ....")
		log.Printf("     ...... MMMM::   o.,OPMP,.o   ::I!! ...")
		log.Printf("         .... NNM:::.,,OOPM!P,.::::!! ....")
		log.Printf("          .. MMNNNNNOOOOPMO!!IIPPO!!O! .....")
		log.Printf("         ... MMMMMNNNNOO:!!:!!IPPPPOO! ....")
		log.Printf("           .. MMMMMNNOOMMNNIIIPPPOO!! ......")
		log.Printf("          ...... MMMONNMMNNNIIIOO!..........")
		log.Printf("       ....... MN MOMMMNNNIIIIIO! OO ..........")
		log.Printf("    ......... MNO! IiiiiiiiiiiiI OOOO ...........")
		log.Printf("  ...... NNN.MNO! . O!!!!!!!!!O . OONO NO! ........")
		log.Printf("   .... MNNNNNO! ...OOOOOOOOOOO .  MMNNON!........")
		log.Printf("   ...... MNNNNO! .. PPPPPPPPP .. MMNON!........")
		log.Printf("      ...... OO! ................. ON! .......")
		log.Printf("         ................................")
		log.Printf("")
	}
	os.Exit(1)
}
