package schwab

import "fmt"

type Session uint8

const (
	SessionNormal   Session = iota // regular trading hours
	SessionAM                      // pre-market session
	SessionPM                      // after-market session
	SessionSeamless                // seamless extended hours
	SessionEXTO                    // extended hours (EXTO)
)

func ParseSession(s string) (Session, error) {
	switch s {
	case "NORMAL":
		return SessionNormal, nil
	case "AM":
		return SessionAM, nil
	case "PM":
		return SessionPM, nil
	case "SEAMLESS":
		return SessionSeamless, nil
	case "EXTO":
		return SessionEXTO, nil
	default:
		return 0, fmt.Errorf("unknown session: %s", s)
	}
}

func (s Session) String() string {
	switch s {
	case SessionNormal:
		return "NORMAL"
	case SessionAM:
		return "AM"
	case SessionPM:
		return "PM"
	case SessionSeamless:
		return "SEAMLESS"
	case SessionEXTO:
		return "EXTO"
	default:
		panic("unknown session")
	}
}

func (s Session) GoString() string {
	switch s {
	case SessionNormal:
		return "SessionNormal"
	case SessionAM:
		return "SessionAM"
	case SessionPM:
		return "SessionPM"
	case SessionSeamless:
		return "SessionSeamless"
	case SessionEXTO:
		return "SessionEXTO"
	default:
		panic("unknown session")
	}
}

func (s Session) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

func (s *Session) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid session: %s", data)
	}
	v, err := ParseSession(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*s = v
	return nil
}
