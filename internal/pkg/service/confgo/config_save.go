package confgo

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/douyu/juno/internal/pkg/service/confgov2"
	"github.com/douyu/juno/internal/pkg/service/resource"
	"github.com/douyu/juno/pkg/model/db"
	"github.com/douyu/juno/pkg/util"
	"github.com/douyu/jupiter/pkg/xlog"
)

func (c *confu) ConfigSaveWorker(from string) (err error) {

	applist, err := resource.Resource.GetAllApp()
	if err != nil {
		return errors.New("GetAllApp: " + err.Error())
	}
	err = c.UpdateGit()
	if err != nil {
		return errors.New("UpdateGit: " + err.Error())
	}
	xlog.Info("config", xlog.String("event", "config_save"), xlog.String("step", "UpdateGit"))

	flag := false
	for _, app := range applist {
		// 安照 appname分类获取配置
		appConfig, err := confgov2.GetAllConfigByAid(app.Aid)
		if err != nil {
			return errors.New("GetAllConfigByAid: " + err.Error())
		}
		if len(appConfig) == 0 {
			xlog.Info("config",
				xlog.String("event", "config_save"),
				xlog.String("step", "config is null"),
				xlog.String("appName", app.AppName),
			)
			continue
		}
		// 将列表进行排序，计算md5值
		sort.Slice(appConfig, func(i, j int) bool {
			if appConfig[i].Zone != appConfig[j].Zone {
				return appConfig[i].Zone < appConfig[j].Zone
			}
			if appConfig[i].Env != appConfig[j].Env {
				return appConfig[i].Env < appConfig[j].Env
			}
			if appConfig[i].Name != appConfig[j].Name {
				return appConfig[i].Name < appConfig[j].Name
			}
			return appConfig[i].Format < appConfig[j].Format
		})
		md5Str := ""
		// 安照 zone_env_name.format的格式准备数据
		data := make(map[string]db.Configuration, 0)
		for _, config := range appConfig {
			md5Str += fmt.Sprintf("%d-%s-%s-%s-%s-%s", config.AID, config.Zone, config.Env, config.Name, config.Format, config.Version)
			key := config.Zone + "_" + config.Env + "_" + config.Name + "." + config.Format
			md5Str += key + "_" + config.Version
			if _, ok := data[key]; !ok {
				data[key] = config
			}
		}
		sum := util.Md5Str(md5Str)
		xlog.Info("config",
			xlog.String("event", "config_save"),
			xlog.String("step", "WriteConfigToFile"),
			xlog.String("appName", app.AppName),
			xlog.Int("len", len(data)),
			xlog.String("md5", sum),
		)

		// 按照 appname分类进行写入
		fileChanged, err := c.WriteConfigToFile(app.AppName, data, sum)
		if err != nil {
			return errors.New("WriteConfigToFile: " + err.Error())
		}

		flag = flag || fileChanged
	}

	if flag { // 如果发生变动，提交git
		msg := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
		xlog.Info("config",
			xlog.String("event", "config_save"),
			xlog.String("step", "PushGit"),
			xlog.String("commit", msg))

		err = c.PushGit(fmt.Sprintf("update by %s,%s", from, msg))
		if err != nil {
			return errors.New("PushGit: " + err.Error())
		}
		return nil
	}
	xlog.Warn("config", xlog.String("event", "config_save"), xlog.String("step", "no change"))

	return nil
}

func (c *confu) WriteConfigToFile(appName string, data map[string]db.Configuration, sum string) (bool, error) {
	if len(data) == 0 {
		return false, nil
	}
	// 检查该appName的文件夹是否存在
	dir := c.GitPath + "/" + appName

	// 检查索引sum文件
	sumFile := dir + "/sum.txt"
	if util.IsExist(sumFile) {
		sumData, err := ioutil.ReadFile(sumFile)
		if err != nil {
			return false, errors.New("readSum: " + err.Error())
		}
		// md5值相等不用写入
		if strings.TrimSpace(string(sumData)) == sum {
			return false, nil
		}
	}

	// 删除appName文件夹下所有文件
	err := os.RemoveAll(dir)
	if err != nil {
		return false, errors.New("RemoveAll: " + err.Error())
	}
	// 创建appName文件夹
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return false, errors.New("MkdirAll: " + err.Error())
	}
	// 写入配置文件
	for fileName, config := range data {
		err = ioutil.WriteFile(dir+"/"+fileName, []byte(config.Content), os.ModePerm)
		if err != nil {
			return false, errors.New("WriteFile: " + err.Error())
		}
	}
	err = ioutil.WriteFile(sumFile, []byte(sum), os.ModePerm)
	if err != nil {
		return false, errors.New("WriteSumFile: " + err.Error())
	}

	return true, nil
}
func (c *confu) UpdateGit() error {
	if !util.IsExist(c.GitPath) {
		// 创建appName文件夹
		err := os.MkdirAll(c.GitPath, os.ModePerm)
		if err != nil {
			return errors.New("MkdirAll: " + err.Error())
		}
	}
	err := os.Chdir(c.GitPath)
	if err != nil {
		return errors.New("Chdir: " + err.Error())
	}
	if !util.IsExist(c.GitPath + "/.git") {
		msg, err := exec.Command("git", "init").Output()
		if err != nil {
			return errors.New("gitInit: " + err.Error() + "," + string(msg))
		}
		msg, err = exec.Command("git", "remote", "add", "origin", c.GitRepo).Output()
		if err != nil {
			return errors.New("gitRemote: " + err.Error() + "," + string(msg))
		}
		msg, err = exec.Command("git", "pull", "origin", "master").Output()
		if err != nil {
			return errors.New("gitPull: " + err.Error() + "," + string(msg))
		}
	} else {
		msg, err := exec.Command("git", "pull", "origin", "master").Output()
		if err != nil {
			return errors.New("gitPull: " + err.Error() + "," + string(msg))
		}
		msg, err = exec.Command("git", "checkout", "HEAD").Output()
		if err != nil {
			return errors.New("gitCheckout: " + err.Error() + "," + string(msg))
		}
	}

	return nil
}

func (c *confu) PushGit(commitMsg string) error {
	msg, err := exec.Command("git", "add", "-A").Output()
	if err != nil {
		return errors.New("gitAdd: " + err.Error() + "," + string(msg))
	}
	msg, err = exec.Command("git", "commit", "-m", commitMsg).Output()
	if err != nil {
		return errors.New("gitCommit: " + err.Error() + "," + string(msg))
	}
	msg, err = exec.Command("git", "push").Output()
	if err != nil {
		return errors.New("gitCommit: " + err.Error() + "," + string(msg))
	}

	return nil

}
