package main

import (
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
)

type user struct {
	Username        string
	selectedAnswers []*pollAnswer
}

func (u *user) SelectAnswers(a []*pollAnswer) {
	if u.selectedAnswers != nil {
		for _, v := range u.selectedAnswers {
			v.Count--
		}
	}
	u.selectedAnswers = a
	for _, v := range u.selectedAnswers {
		v.Count++
	}
}

type pollAnswer struct {
	GUID  string
	Text  string
	Count int
}

type poll struct {
	GUID        string
	Title       string
	Multiselect bool
	Answers     []pollAnswer
	Users       []*user
	CreatedBy   string

	m sync.Mutex
}

func (p *poll) GetUser(name string) *user {
	p.m.Lock()
	defer p.m.Unlock()

	for _, u := range p.Users {
		if u.Username == name {
			return u
		}
	}

	u := &user{
		Username: name,
	}

	p.Users = append(p.Users, u)
	return u
}

type polls struct {
	polls []*poll
	m     sync.RWMutex
}

func (p *polls) Add(n *poll) {
	p.m.Lock()
	defer p.m.Unlock()
	p.polls = append(p.polls, n)
}

func (p *polls) Get(guid string) *poll {
	p.m.RLock()
	defer p.m.RUnlock()

	for i := 0; i < len(p.polls); i++ {
		if p.polls[i].GUID == guid {
			return p.polls[i]
		}
	}
	return nil
}

var localPolls polls = polls{polls: []*poll{}}

func main() {
	ssh.Handle(func(s ssh.Session) {
		mainHandler(s)
	})

	log.Print("Server starting on port 2222. You can connect by using 'ssh -p 2222 127.0.0.1'")

	log.Fatal(ssh.ListenAndServe(":2222", nil))
}

func mainHandler(s ssh.Session) {
	for {
		writeMainMenu(s)
		k, err := readKey(s)
		if err != nil {
			io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
			return
		}

		switch string(k) {
		case "c":
			createHandler(s)
		case "o":
			openHandler(s)
		case "h":
			writeMainMenu(s)
		case "x":
			writeBye(s)
			s.Close()
		}
	}
}

func createHandler(s ssh.Session) {
	io.WriteString(s, "\nPoll Title: ")
	t, err := readLine(s)
	if err != nil {
		io.WriteString(s, "\nCannot read input: "+err.Error()+"\n")
		return
	}

	io.WriteString(s, "\nMultiselect (y/n): ")
	k, err := readKey(s)
	if err != nil {
		io.WriteString(s, "\nCannot read input: "+err.Error()+"\n")
		return
	}

	if string(k) == "y" {
		io.WriteString(s, "Yes\n")
	} else {
		io.WriteString(s, "No\n")
	}

	p := poll{
		GUID:        uuid.NewString(),
		Title:       t,
		Multiselect: string(k) == "y",
		Answers:     []pollAnswer{},
		Users:       []*user{},
		CreatedBy:   s.User(),
	}

	io.WriteString(s, "Please Enter the answers one per line. Empty line ends creation phase.\n")
	for {
		a, err := readLine(s)
		if err != nil {
			io.WriteString(s, "\nCannot read input: "+err.Error()+"\n")
			return
		}
		if a == "" {
			break
		}
		p.Answers = append(p.Answers, pollAnswer{
			GUID: uuid.NewString(),
			Text: a,
		})
	}

	localPolls.Add(&p)

	io.WriteString(s, fmt.Sprintf("\nCreated Poll \"%s\".\nGUID is %s\nGive this GUID to others so they can answer your poll.\nPress any key to go back to the main menu.\n", p.Title, p.GUID))
	_, _ = readKey(s)
}

func openHandler(s ssh.Session) {
	io.WriteString(s, "\nPlease enter Poll-GUID: ")
	g, err := readLine(s)
	if err != nil {
		io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
		return
	}

	p := localPolls.Get(g)
	if p == nil {
		io.WriteString(s, "No Poll with this GUID found. Returning to main menu.\n")
		return
	}

	writePoll(s, p)

	user := p.GetUser(s.User())

	if user.selectedAnswers == nil {
		if p.Multiselect {
			io.WriteString(s, "Multiple selections are possible. Please enter the numbers of your choices as separated by commas and confirm with enter.\n")
			inp, err := readLine(s)
			if err != nil {
				io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
				return
			}
			c := strings.Split(inp, ",")
			answers := []*pollAnswer{}
			for _, v := range c {
				i64, err := strconv.ParseInt(v, 10, 32)
				i := int(i64)
				if err != nil {
					io.WriteString(s, fmt.Sprintf("%s is no valid number: %s", v, err.Error()))
					return
				}
				if i < 0 || i >= len(p.Answers) {
					io.WriteString(s, fmt.Sprintf("%s is out of bounds", v))
					return
				}
				answers = append(answers, &p.Answers[i])
			}

			user.SelectAnswers(answers)
		} else {
			io.WriteString(s, "Please enter the number of your choice and confirm with enter.\n")
			inp, err := readLine(s)
			if err != nil {
				io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
				return
			}
			i64, err := strconv.ParseInt(inp, 10, 32)
			i := int(i64)
			if err != nil {
				io.WriteString(s, fmt.Sprintf("%s is no valid number: %s", inp, err.Error()))
				return
			}
			if i < 0 || i >= len(p.Answers) {
				io.WriteString(s, fmt.Sprintf("%s is out of bounds", inp))
				return
			}
			user.SelectAnswers([]*pollAnswer{&p.Answers[i]})
		}

		writePoll(s, p)
	} else {
		io.WriteString(s, "You already voted, so here are the results\n")
	}

	for {
		io.WriteString(s, " --- Press r to refresh or x to exit to the main menu ---\n")
		k, err := readKey(s)
		if err != nil {
			io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
			return
		}

		switch string(k) {
		case "r":
			writePoll(s, p)
		case "x":
			return
		}
	}
}

func writePoll(s ssh.Session, p *poll) {
	io.WriteString(s, fmt.Sprintf("\f******************************************************\nTitle: %s\nCreated by %s\n\n", p.Title, p.CreatedBy))
	for i := 0; i < len(p.Answers); i++ {
		io.WriteString(s, fmt.Sprintf(" %d. %s (%d votes)\n", i, p.Answers[i].Text, p.Answers[i].Count))
	}

	if s.User() == p.CreatedBy {
		usernames := []string{}
		for _, u := range p.Users {
			usernames = append(usernames, u.Username)
		}
		io.WriteString(s, fmt.Sprintf("The following users have voted: %s\n", strings.Join(usernames, ", ")))
	} else {
		io.WriteString(s, fmt.Sprintf("%d users have voted\n", len(p.Users)))
	}

	io.WriteString(s, "\n")
}

func writeMainMenu(s ssh.Session) {
	io.WriteString(s, fmt.Sprintf("\fHello %s,\navailable commands are:\n- (c)reate new poll\n- (o)pen existing poll\n- (h)elp\n- e(x)it\n\n", s.User()))
}

func writeBye(s io.Writer) {
	io.WriteString(s, "\fSee you later...\n\n\n")
}

func readLine(s ssh.Session) (string, error) {
	result := ""
	buf := make([]byte, 1)

	for {
		_, err := s.Read(buf)
		if err != nil {
			return "", fmt.Errorf("could not read from session: %w", err)
		}
		switch buf[0] {
		case 127: // backspace
			if result != "" {
				result = result[:len(result)-1]
				io.WriteString(s, "\b \b")
			}
		case 13: // enter
			io.WriteString(s, "\n")
			return result, nil
		default:
			io.WriteString(s, string(buf))
			result += string(buf)
		}
	}
}

func readKey(s ssh.Session) (byte, error) {
	buf := make([]byte, 1)
	_, err := s.Read(buf)
	if err != nil {
		return buf[0], fmt.Errorf("could not read from session: %w", err)
	}
	return buf[0], nil
}
