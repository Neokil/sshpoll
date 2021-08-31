package pollserver

import (
	"fmt"
	"io"
	"sshpoll/internal/sshio"
	"strconv"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
)

type PollServer interface {
	Handler(s ssh.Session)
}

type server struct {
	polls polls
}

func New() PollServer {
	return &server{polls: polls{polls: []*poll{}}}
}

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

func (srv *server) Handler(s ssh.Session) {
	srv.mainMenuHandler(s)
}

func (srv *server) mainMenuHandler(s ssh.Session) {
	for {
		srv.writeMainMenu(s)
		k, err := sshio.ReadKey(s)
		if err != nil {
			_, _ = io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
			return
		}

		switch string(k) {
		case "c":
			srv.createHandler(s)
		case "o":
			srv.openHandler(s)
		case "h":
			srv.writeMainMenu(s)
		case "x":
			srv.writeBye(s)
			s.Close()
		}
	}
}

func (srv *server) createHandler(s ssh.Session) {
	_, _ = io.WriteString(s, "\nPoll Title: ")
	t, err := sshio.ReadLine(s)
	if err != nil {
		_, _ = io.WriteString(s, "\nCannot read input: "+err.Error()+"\n")
		return
	}

	_, _ = io.WriteString(s, "\nMultiselect (y/n): ")
	k, err := sshio.ReadKey(s)
	if err != nil {
		_, _ = io.WriteString(s, "\nCannot read input: "+err.Error()+"\n")
		return
	}

	if string(k) == "y" {
		_, _ = io.WriteString(s, "Yes\n")
	} else {
		_, _ = io.WriteString(s, "No\n")
	}

	p := poll{
		GUID:        uuid.NewString(),
		Title:       t,
		Multiselect: string(k) == "y",
		Answers:     []pollAnswer{},
		Users:       []*user{},
		CreatedBy:   s.User(),
	}

	_, _ = io.WriteString(s, "Please Enter the answers one per line. Empty line ends creation phase.\n")
	for {
		a, err := sshio.ReadLine(s)
		if err != nil {
			_, _ = io.WriteString(s, "\nCannot read input: "+err.Error()+"\n")
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

	srv.polls.Add(&p)

	_, _ = io.WriteString(s, fmt.Sprintf("\nCreated Poll \"%s\".\nGUID is %s\nGive this GUID to others so they can answer your poll.\nPress any key to go back to the main menu.\n", p.Title, p.GUID))
	_, _ = sshio.ReadKey(s)
}

func (srv *server) openHandler(s ssh.Session) {
	_, _ = io.WriteString(s, "\nPlease enter Poll-GUID: ")
	g, err := sshio.ReadLine(s)
	if err != nil {
		_, _ = io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
		return
	}

	p := srv.polls.Get(g)
	if p == nil {
		_, _ = io.WriteString(s, "No Poll with this GUID found. Returning to main menu.\n")
		return
	}

	srv.writePoll(s, p)

	user := p.GetUser(s.User())

	if user.selectedAnswers == nil {
		srv.handlePollAnswering(s, p, user)
		srv.writePoll(s, p)
	} else {
		_, _ = io.WriteString(s, "You already voted, so here are the results\n")
	}

	for {
		_, _ = io.WriteString(s, " --- Press r to refresh or x to exit to the main menu ---\n")
		k, err := sshio.ReadKey(s)
		if err != nil {
			_, _ = io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
			return
		}

		switch string(k) {
		case "r":
			srv.writePoll(s, p)
		case "x":
			return
		}
	}
}

func (srv *server) handlePollAnswering(s ssh.Session, p *poll, user *user) {
	if p.Multiselect {
		_, _ = io.WriteString(s, "Multiple selections are possible. Please enter the numbers of your choices as separated by commas and confirm with enter.\n")
		inp, err := sshio.ReadLine(s)
		if err != nil {
			_, _ = io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
			return
		}
		c := strings.Split(inp, ",")
		answers := []*pollAnswer{}
		for _, v := range c {
			i64, err := strconv.ParseInt(v, 10, 32)
			i := int(i64)
			if err != nil {
				_, _ = io.WriteString(s, fmt.Sprintf("%s is no valid number: %s", v, err.Error()))
				return
			}
			if i < 0 || i >= len(p.Answers) {
				_, _ = io.WriteString(s, fmt.Sprintf("%s is out of bounds", v))
				return
			}
			answers = append(answers, &p.Answers[i])
		}

		user.SelectAnswers(answers)
		return
	}

	_, _ = io.WriteString(s, "Please enter the number of your choice and confirm with enter.\n")
	inp, err := sshio.ReadLine(s)
	if err != nil {
		_, _ = io.WriteString(s, "Cannot read input: "+err.Error()+"\n")
		return
	}
	i64, err := strconv.ParseInt(inp, 10, 32)
	i := int(i64)
	if err != nil {
		_, _ = io.WriteString(s, fmt.Sprintf("%s is no valid number: %s", inp, err.Error()))
		return
	}
	if i < 0 || i >= len(p.Answers) {
		_, _ = io.WriteString(s, fmt.Sprintf("%s is out of bounds", inp))
		return
	}
	user.SelectAnswers([]*pollAnswer{&p.Answers[i]})
}

func (srv *server) writePoll(s ssh.Session, p *poll) {
	pty, _, ok := s.Pty()
	if ok {
		if err := sshio.NewPage(s, pty.Window.Width, pty.Window.Height); err != nil {
			_, _ = io.WriteString(s, err.Error()+"\n")
		}
	}

	_, _ = io.WriteString(s, fmt.Sprintf("\f******************************************************\nTitle: %s\nCreated by %s\n\n", p.Title, p.CreatedBy))
	for i := 0; i < len(p.Answers); i++ {
		_, _ = io.WriteString(s, fmt.Sprintf(" %d. %s (%d votes)\n", i, p.Answers[i].Text, p.Answers[i].Count))
	}

	if s.User() == p.CreatedBy {
		usernames := []string{}
		for _, u := range p.Users {
			usernames = append(usernames, u.Username)
		}
		_, _ = io.WriteString(s, fmt.Sprintf("The following users have voted: %s\n", strings.Join(usernames, ", ")))
	} else {
		_, _ = io.WriteString(s, fmt.Sprintf("%d users have voted\n", len(p.Users)))
	}

	_, _ = io.WriteString(s, "\n")
}

func (srv *server) writeMainMenu(s ssh.Session) {
	pty, _, ok := s.Pty()
	if ok {
		if err := sshio.NewPage(s, pty.Window.Width, pty.Window.Height); err != nil {
			_, _ = io.WriteString(s, err.Error()+"\n")
		}
	}
	_, _ = io.WriteString(s, fmt.Sprintf("\fHello %s,\navailable commands are:\n- (c)reate new poll\n- (o)pen existing poll\n- (h)elp\n- e(x)it\n\n", s.User()))
}

func (srv *server) writeBye(s ssh.Session) {
	pty, _, ok := s.Pty()
	if ok {
		if err := sshio.NewPage(s, pty.Window.Width, pty.Window.Height); err != nil {
			_, _ = io.WriteString(s, err.Error()+"\n")
		}
	}
	_, _ = io.WriteString(s, "\fSee you later...\n\n\n")
}
