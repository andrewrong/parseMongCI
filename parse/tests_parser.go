package parse

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// tests.log的分析任务
type TestParseTask struct {
	TaskKeys    []string
	TestFileDir string //存放错误日志的文件的目录
	TestLogPath string
}

func NewTestParseTask(keys []string, testFileDir, testLogPath string) *TestParseTask {
	if len(keys) == 0 || len(testFileDir) == 0 || len(testLogPath) == 0 {
		return nil
	}

	tmp := TestParseTask{
		TaskKeys:    keys,
		TestFileDir: testFileDir,
		TestLogPath: testLogPath,
	}

	return &tmp
}

func (this *TestParseTask) Run() error {
	fileManager := make(map[string]*bufio.Writer)

	for _, key := range this.TaskKeys {
		if len(key) == 0 {
			log.Printf("%s is empty", key)
			continue
		}

		tmpSlice := strings.Split(key, ":")
		suiteName := tmpSlice[1]
		if _, err := os.Stat(this.TestFileDir + "/" + suiteName); os.IsNotExist(err) {
			err = os.Mkdir(this.TestFileDir+"/"+suiteName, os.ModePerm)
			if err != nil {
				log.Printf("create dir is error:%s", err)
				return err
			}
		}

		tmp, err := os.OpenFile(this.TestFileDir+"/"+suiteName+"/"+key, os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			log.Printf("open key:%s is error: %s", key, err)
			return err
		}
		defer tmp.Close()

		writer := bufio.NewWriter(tmp)
		fileManager[key] = writer
	}

	testFile, err := os.Open(this.TestLogPath)
	if err != nil {
		log.Printf("open test log:%s is error: %s", this.TestLogPath, err)
		return err
	}
	defer testFile.Close()

	scanner := bufio.NewScanner(testFile)
	const maxCap = 10 * 1024 * 1024
	buf := make([]byte, maxCap)
	scanner.Buffer(buf, maxCap)
	globalLine := ""
	for scanner.Scan() {
		line := scanner.Text()
		globalLine = line
		for _, key := range this.TaskKeys {
			if len(key) == 0 {
				continue
			}

			if strings.Contains(line, key) {
				_, err = fileManager[key].WriteString(line + "\n")
				if err != nil {
					log.Printf("write key file:%s is error: %s", key, err)
					return err
				}
				break
			}
		}
	}

	for _, item := range fileManager {
		item.Flush()
	}

	if scanner.Err() != nil {
		log.Printf("scanner is error: %s", scanner.Err())
		log.Printf("the last line:%s", globalLine)
		return scanner.Err()
	}

	return nil
}
