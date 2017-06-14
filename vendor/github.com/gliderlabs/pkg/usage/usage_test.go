package usage

import (
	"fmt"
	"log"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func Test_RequestLatest(t *testing.T) {
	pv := &ProjectVersion{"registrator", "v1"}
	latest, err := RequestLatest(pv)
	ok(t, err)
	log.Println(latest)
}

func Test_FormatV1(t *testing.T) {
	pv := &ProjectVersion{"registrator", "v1.0.0"}
	act := FormatV1(pv)
	equals(t, "v1.0.0.registrator.usage-v1.", act)
}

func Test_ParseV1_Success(t *testing.T) {
	pv, err := ParseV1("v1.0.0.registrator.usage-v1.")
	ok(t, err)
	equals(t, "registrator", pv.Project)
	equals(t, "v1.0.0", pv.Version)
}

func Test_ParseV1_MissingSuffix(t *testing.T) {
	_, err := ParseV1("v1.0.0.registrator.")
	if err == nil {
		t.Fatal("expected an error, but got nil")
	}
}

func Test_ParseV1_MissingVersion(t *testing.T) {
	_, err := ParseV1("registrator.usage-v1.")
	if err == nil {
		t.Fatal("expected an error, but got nil")
	}
}

func Test_ParseV1_EmptyVersion(t *testing.T) {
	_, err := ParseV1(".registrator.usage-v1.")
	if err == nil {
		t.Fatal("expected an error, but got nil")
	}
}

func Test_ParseV1_EmptyProject(t *testing.T) {
	_, err := ParseV1("v1.0.0..usage-v1.")
	if err == nil {
		t.Fatal("expected an error, but got nil")
	}
}

// ok fails the test if an err is not nil.
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// equals fails the test if exp is not equal to act.
func equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}
