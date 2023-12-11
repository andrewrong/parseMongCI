package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"

	"./parse"
)

var resultFileName = "executor.log"
var testFileName = "tests.log"
var dbPath string
var jobs int
var basePort int
var cmp bool

func main() {
	var oldCIPath string
	var newCIPath string
	flag.StringVar(&oldCIPath, "oldCI", "./", "老的测试集合的路径")
	flag.StringVar(&newCIPath, "newCI", "./", "新的测试集合的路径")
	flag.StringVar(&dbPath, "dbPath", "./", "集成测试的db路径")
	flag.IntVar(&jobs, "jobs", 10, "jobs")
	flag.IntVar(&basePort, "basePort", 20000, "basePort")
	flag.BoolVar(&cmp, "cmp", false, "old/new比较模式")

	flag.Parse()

	log.Printf("oldCIPath:%s\n", oldCIPath)
	log.Printf("newCIPath:%s\n", newCIPath)
	log.Printf("dbPath:%s\n", dbPath)
	log.Printf("jobs:%v\n", jobs)
	log.Printf("basePort:%v\n", basePort)
	log.Printf("cmp:%v\n", cmp)

	if len(dbPath) == 0 || jobs <= 0 || basePort < 20000 || len(newCIPath) == 0 {
		log.Println("params is invalid")
		return
	}

	if cmp && len(oldCIPath) == 0 {
		log.Println("oldCIPath is invalid")
	}

	newTestResult, newTestParseTask, err := getTestResultInfos(newCIPath, dbPath, jobs, basePort, false)
	if err != nil {
		log.Printf("get new test Result is error: %v", err)
		return
	}

	log.Printf("retry cmd:%s", newTestResult.GetNotRunningRestartCmd())
	contents, err := json.Marshal(newTestResult.GetFailureSuites())

	if err != nil {
		log.Printf("json error:%s", err)
		return
	}
	log.Printf("FailureSuites:" + string(contents))

	err = newTestParseTask.Run()
	if err != nil {
		log.Printf("new Test parse task is error: %v", err)
		return
	}

	if cmp {
		oldTestResult, _, err := getTestResultInfos(oldCIPath, dbPath, jobs, basePort, false)
		if err != nil {
			log.Printf("get old test Result is error: %v", err)
			return
		}
		newTestResult.DiffOtherResult(oldTestResult)
	}
}

func getTestResultInfos(ciPath, dbPath string, jobs, basePort int, old bool) (*parse.TestReult, *parse.TestParseTask, error) {
	//判断路径是否存在
	if _, err := os.Stat(ciPath); os.IsNotExist(err) {
		log.Printf("%s is not exist; err:%v", ciPath, err)
		return nil, nil, err
	}

	resultFile := ciPath + "/" + resultFileName
	testFile := ciPath + "/" + testFileName

	if _, err := os.Stat(resultFile); os.IsNotExist(err) {
		log.Printf("%s is not exist; err:%v", resultFileName, err)
		return nil, nil, err
	}

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		log.Printf("%s is not exist; err:%v", testFileName, err)
		return nil, nil, err
	}

	testResult := parse.NewTestResult(resultFile, dbPath, int32(jobs), int32(basePort))
	if testResult == nil {
		return nil, nil, errors.New("init TestResult is nil")
	}
	err := testResult.ParseTestResult()
	if err != nil {
		log.Printf("ParseTestResult is err:%v", err)
		return nil, nil, err
	}

	if old {
		return testResult, nil, nil
	}

	testDetailPath := ciPath + "/test_detail"

	errKeys := testResult.GetFailedKeys()
	if _, err := os.Stat(testDetailPath); !os.IsNotExist(err) {
		err := os.RemoveAll(testDetailPath)
		if err != nil {
			log.Printf("delete dir is error:%s", err)
			return testResult, nil, err
		}
	}
	err = os.Mkdir(testDetailPath, os.ModePerm)
	if err != nil {
		log.Printf("create dir is err:%s", err)
		return testResult, nil, err
	}

	parseTestTask := parse.NewTestParseTask(errKeys, testDetailPath, testFile)
	err = parseTestTask.Run()
	if err != nil {
		log.Printf("TestParseTask run is error")
		return testResult, nil, err
	}
	return testResult, parseTestTask, nil
}
