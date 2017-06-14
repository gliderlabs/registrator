package usage

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

const dnsHost = "usage.gliderlabs.io:53"

type ProjectVersion struct {
	Project string
	Version string
}

func RequestLatest(pv *ProjectVersion) (*ProjectVersion, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(FormatV1(pv), dns.TypePTR)
	in, err := dns.Exchange(msg, dnsHost)
	if err != nil {
		return nil, err
	}
	for _, ans := range in.Answer {
		if ptr, ok := ans.(*dns.PTR); ok {
			return ParseV1(ptr.Ptr)
		}
	}
	return nil, errors.New("no answer found")
}

type checkResult struct {
	pv  *ProjectVersion
	err error
}

type Checker struct {
	Current  *ProjectVersion
	result   checkResult
	resultCh chan checkResult
	once     sync.Once
}

var CheckDisabledError = errors.New("Version check disabled by GL_DISABLE_VERSION_CHECK")

func NewChecker(project, version string) *Checker {
	current := ProjectVersion{project, version}
	checker := Checker{Current: &current}

	if os.Getenv("GL_DISABLE_VERSION_CHECK") != "" {
		checker.once.Do(func() {
			checker.result.err = CheckDisabledError
		})
	} else {
		resultCh := make(chan checkResult, 1)
		checker.resultCh = resultCh

		go func() {
			latest, err := RequestLatest(&current)
			resultCh <- checkResult{latest, err}
			close(resultCh)
		}()
	}

	return &checker
}

func (c *Checker) Latest() (string, error) {
	c.once.Do(func() {
		c.result = <-c.resultCh
	})
	if c.result.err != nil {
		return "", c.result.err
	}
	return c.result.pv.Version, nil
}

func (c *Checker) PrintVersion() {
	fmt.Println(c.Current.Version)
	latest, err := c.Latest()
	if err == nil && latest != c.Current.Version {
		fmt.Printf("\nYour %s version is out of date!\nThe latest version is %s\n", c.Current.Project, latest)
	}
}

func ParseV1(domain string) (*ProjectVersion, error) {
	prefix := strings.TrimSuffix(domain, ".usage-v1.")
	if len(prefix) == len(domain) {
		return nil, errors.New("should end in '.usage-v1.'")
	}

	lastDot := strings.LastIndex(prefix, ".")
	if lastDot < 0 {
		return nil, errors.New("missing '.' separator")
	}

	version := prefix[:lastDot]
	project := prefix[lastDot+1:]

	if len(version) == 0 {
		return nil, errors.New("version should not be empty")
	}
	if len(project) == 0 {
		return nil, errors.New("project should not be empty")
	}

	return &ProjectVersion{project, version}, nil
}

func FormatV1(pv *ProjectVersion) string {
	return fmt.Sprintf("%s.%s.usage-v1.", pv.Version, pv.Project)
}
