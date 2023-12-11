package parse

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var ResultBeginKey = "Summary of all suites:"
var StatusKeys = []string{"succeed", "skipped", "failed", "errored"}
var JsTests = "js_tests"
var JsTestsKey = "jstests/"
var NotRunAny = "not run any tests"

type Status = string

var Failed Status = "failed"
var Success Status = "success"
var NotRun Status = "NotRunAny"

type TestReult struct {
	CS       float64                // 整体耗时
	Results  map[string]*SuitResult //每个suit的结果
	SuiteCnt int32
	fName    string
	DBPath   string
	Jobs     int32
	BasePort int32
}

func NewTestResult(fname, dbPath string, jobs, basePort int32) *TestReult {
	if len(fname) == 0 || len(dbPath) == 0 || jobs <= 0 || basePort < 20000 {
		return nil
	}
	if _, err := os.Stat(fname); os.IsNotExist(err) {
		log.Printf("%s is not existed, err:%v", fname, err)
		return nil
	}

	tmp := TestReult{
		fName:    fname,
		Results:  make(map[string]*SuitResult),
		DBPath:   dbPath,
		Jobs:     jobs,
		BasePort: basePort,
	}
	return &tmp
}

//两次测试结果一致
func (this *TestReult) DiffOtherResult(other *TestReult) {
	if this.SuiteCnt != other.SuiteCnt {
		log.Printf("self suiteCnt:%v, other sutieCnt:%v", this.SuiteCnt, other.SuiteCnt)
		return
	}

	for key, value := range this.Results {
		if tmp, ok := other.Results[key]; ok {
			if value.Same(tmp) {
				log.Printf("suite_name:%s self == other", key)
				continue
			} else {
				log.Printf("suite_name:%s self != other", key)
			}
		} else {
			log.Printf("self %v is failure, but other %v is not existed", key, key)
		}
	}
}

func (this *TestReult) ToString() string {
	var build strings.Builder

	build.WriteString("测试Suites:" + strconv.Itoa(int(this.SuiteCnt)) + "\n")
	build.WriteString("测试耗时:" + fmt.Sprintf("%f", this.CS) + "\n")
	for k, v := range this.Results {
		build.WriteString("suiteName:" + k + ", result:" + v.FinalResult + "\n")
	}

	return build.String()
}

func (this *TestReult) GetFailure() string {
	var build strings.Builder
	for k, v := range this.Results {
		if v.FinalResult != Success {
			build.WriteString("suiteName:" + k + ", result:" + v.FinalResult + "\n")
		}
	}

	return build.String()
}

func (this *TestReult) GetFailureSuites() []*SuitResult {
	results := make([]*SuitResult, 0)
	for _, v := range this.Results {
		if v.FinalResult != Success && v.FinalResult != NotRun {
			results = append(results, v)
		}
	}
	return results
}

func (this *TestReult) GetNotRunningRestartCmd() string {
	var build strings.Builder
	build.WriteString("nohup python buildscripts/resmoke.py --suites=")

	suits := make([]string, 0)
	for k, v := range this.Results {
		if v.FinalResult == NotRun {
			suits = append(suits, k)
		}
	}
	build.WriteString(strings.Join(suits, ","))
	build.WriteString(" --continueOnFailure --log=file --dbpathPrefix=")
	build.WriteString(this.DBPath)
	build.WriteString(" --jobs=")
	build.WriteString(strconv.Itoa(int(this.Jobs)))
	build.WriteString(" --basePort=")
	build.WriteString(strconv.Itoa(int(this.BasePort)))
	build.WriteString(" > log&")

	return build.String()
}

func (this *TestReult) GetFailedKeys() []string {
	keys := make([]string, 0)
	for _, v := range this.Results {
		if v.FinalResult != Success {
			for _, errItem := range v.ErrList {
				if len(errItem) == 0 {
					continue
				}
				baseFile := path.Base(errItem)
				tmpSegs := strings.Split(baseFile, ".")

				if v.TestType == "js_tests" {
					keys = append(keys, fmt.Sprintf("[%s:%s:%s]", "js_test", v.Name, tmpSegs[0]))
				} else {
					keys = append(keys, fmt.Sprintf("[%s:%s:%s]", "other", v.Name, tmpSegs[0]))
				}
			}
		}
	}
	return keys
}

func (this *TestReult) ParseTestResult() error {
	testFile, err := os.Open(this.fName)
	if err != nil {
		return err
	}
	defer testFile.Close()

	scanner := bufio.NewScanner(testFile)
	const maxCap = 64 * 1024
	buf := make([]byte, maxCap)
	scanner.Buffer(buf, maxCap)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ResultBeginKey) {
			// 6: suite con
			// 10: cs seconds
			// 11: seonds key
			segs := strings.Split(line, " ")

			if len(segs) != 12 || segs[11] != "seconds" {
				log.Printf("TestResult is invalid, content:%v", line)
				return errors.New("TestResult is invalid")
			}

			suiteCnt, err := strconv.Atoi(segs[6])
			if err != nil {
				log.Printf("suiteCnt is error, content:%v", line)
				return errors.New("TestResult is invalid")
			}
			this.SuiteCnt = int32(suiteCnt)

			cs, err := strconv.ParseFloat(segs[10], 10)
			if err != nil {
				log.Printf("cs is error, content:%v", line)
				return errors.New("TestResult is invalid")
			}
			this.CS = cs
			break
		}
	}

	if this.SuiteCnt == 0 {
		return errors.New("don't find Result Key")
	}

	suiteLines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		//判断是否是一个suite的开始;

		if this.judgeNewSuite(line) {
			if len(suiteLines) != 0 {
				suiteTmp, err := this.parseOneSuit(suiteLines)
				if err != nil {
					return err
				}
				this.Results[suiteTmp.Name] = suiteTmp
				suiteLines = make([]string, 0)
			}
		}
		suiteLines = append(suiteLines, line)
	}

	return nil
}

func (this *TestReult) judgeNewSuite(line string) bool {
	if strings.Contains(line, NotRunAny) {
		return true
	}

	if !strings.Contains(line, JsTests) {
		valid := true
		for _, key := range StatusKeys {
			if strings.Contains(line, key) {
				continue
			} else {
				valid = false
				break
			}
		}
		return valid
	}

	return false
}

//传入的是整个
func (this *TestReult) parseOneSuit(lines []string) (*SuitResult, error) {
	first := lines[0]
	re := regexp.MustCompile(`\s+`)
	segments := re.Split(first, -1)
	tmp := NewSuitResult(segments[1][:len(segments[1])-1])
	if strings.Contains(first, NotRunAny) {
		tmp.FinalResult = NotRun
		return tmp, nil
	}

	/**
	 * 第一行可能会带有的信息
	 * 1: name
	 * 2: test cnt,
	 * 6: consume time seconds
	 * 7: seconds
	 * 8: success cnt, (1777
	 * 9: succeeded,
	 * 10: skip cnt
	 * 12: skipped,
	 * 13: failed cnt,
	 * 14: failed,
	 * 15: errored con
	 * 16: errored)
	 */

	if segments[7] != "seconds" || segments[9] != "succeeded," || segments[12] != "skipped," || segments[14] != "failed," || segments[16] != "errored)" {
		log.Printf("this first line is not standard, line:%v", first)
		return nil, errors.New("this first line is not standard")
	}

	testCnt, err := strconv.Atoi(segments[2])
	if err != nil {
		log.Printf("test cnt is error, err:%v, line:%s", err, first)
		return nil, err
	}
	tmp.TestCnt = int32(testCnt)
	cs, err := strconv.ParseFloat(segments[6], 10)
	if err != nil {
		log.Printf("consume time is error, err:%v, line:%s", err, first)
		return nil, err
	}
	tmp.CS = cs

	succCnt, err := strconv.Atoi(segments[8][1:])
	if err != nil {
		log.Printf("succCnt is error, err:%v, line:%s", err, first)
		return nil, err
	}
	tmp.SuccessCnt = int32(succCnt)

	skippedCnt, err := strconv.Atoi(segments[10])
	if err != nil {
		log.Printf("skipped is error, err:%v, line:%s", err, first)
		return nil, err
	}
	tmp.SkippedCnt = int32(skippedCnt)

	failCnt, err := strconv.Atoi(segments[13])
	if err != nil {
		log.Printf("failCnt is error, err:%v, line:%s", err, first)
		return nil, err
	}
	tmp.FailedCnt = int32(failCnt)

	errorCnt, err := strconv.Atoi(segments[15])
	if err != nil {
		log.Printf("errorCnt is error, err:%v, line:%s", err, first)
		return nil, err
	}
	tmp.ErrorCnt = int32(errorCnt)

	if tmp.ErrorCnt == 0 && tmp.FailedCnt == 0 && tmp.SkippedCnt == 0 {
		tmp.FinalResult = Success
	} else {
		tmp.FinalResult = Failed
	}

	if tmp.FinalResult == Success {
		return tmp, nil
	}

	if len(lines) <= 3 {
		return nil, errors.New("lines len is invalid")
	}

	second := lines[1]
	segsSecond := re.Split(second, -1)
	if len(segsSecond[1]) != 0 {
		tmp.TestType = segsSecond[1][0 : len(segsSecond[1])-1]
	}
	//处理有问题的测试用例
	for i := 3; i < len(lines); i++ {
		if strings.Contains(lines[i], "The following tests") {
			continue
		}

		segs := re.Split(lines[i], -1)
		if len(segs) != 3 {
			log.Printf("error jstest is invalid, len != 3, content:%v", lines[i])
			//return nil, errors.New("error jstest invalid")
			log.Printf("%++v", segs)
		}
		if len(segs[1]) != 0 {
			tmp.AddErrList(segs[1])
		} else {
			log.Printf("jstest name is empty, content:%v", lines[i])
			return nil, errors.New("jstest name is empty")
		}
	}
	return tmp, nil
}

// 每一个Suit的测试结果集
type SuitResult struct {
	Name        string
	FinalResult Status
	CS          float64
	TestCnt     int32
	SuccessCnt  int32
	SkippedCnt  int32
	FailedCnt   int32
	ErrorCnt    int32
	ErrList     []string //有问题的测试列表
	TestType    string
}

func NewSuitResult(name string) *SuitResult {
	if len(name) == 0 {
		return nil
	}

	tmp := SuitResult{
		Name:    name,
		ErrList: make([]string, 0),
	}
	return &tmp
}

func (this *SuitResult) Same(other *SuitResult) bool {
	if this.FinalResult != other.FinalResult {
		log.Printf("name:%s final result is not equal, self:%s, other:%s", this.Name, this.FinalResult, other.FinalResult)
		return false
	}

	if this.SuccessCnt != other.SuccessCnt || this.SkippedCnt != other.SkippedCnt || this.FailedCnt != other.FailedCnt || this.ErrorCnt != other.ErrorCnt {
		log.Printf("name:%s count is not equal; self:(%v, %v, %v, %v) != other:(%v, %v, %v, %v)", this.Name, this.SuccessCnt, this.FailedCnt, this.SkippedCnt, this.ErrorCnt, other.SuccessCnt, other.FailedCnt, other.SkippedCnt, other.ErrorCnt)
		return false
	}

	sort.Strings(this.ErrList)
	sort.Strings(other.ErrList)

	if reflect.DeepEqual(this.ErrList, other.ErrList) {
		return true
	} else {
		log.Printf("name:%s, errList is not equal, self:%v != other:%v", this.Name, this.ErrList, other.ErrList)
		return false
	}
}

func (this *SuitResult) AddErrList(suit string) {
	if len(suit) == 0 {
		return
	}
	this.ErrList = append(this.ErrList, suit)
}
