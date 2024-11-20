package internal

import (
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"

	"github.com/robertkrimen/otto"
	log "github.com/sirupsen/logrus"
)

var client http.Client
var vm *otto.Otto
var systemData SystemData
var portData []PortData

type Collector struct {
	address  string
	password string
}

type SystemData struct {
	Max_port    int64
	model_name  string
	sys_fmw_ver string
	sys_IP      string
	sys_MAC     string
	loop        string
	vlans       []string
}

type PortStats struct {
	rx float64
	tx float64
}

type PortData struct {
	name        string
	loop_status string
	portstatus  string
	speed       int
	speedUnit   string
	duplex      string
	stats       PortStats
	pvlan       string
	vlans       []string
}

func GS1200Collector(address string, password string) (*Collector, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	client = http.Client{
		Jar: jar,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			MaxIdleConns:      5,
		},
	}

	collector := &Collector{
		address:  address,
		password: password,
	}
	return collector, nil
}

func (c *Collector) GetValue(name string) otto.Value {
	value, _ := vm.Get(name)
	return value
}

func (c *Collector) GetFloat(name string) float64 {
	value, _ := c.GetValue(name).ToFloat()
	return value
}

func (c *Collector) GetString(name string) string {
	value, _ := c.GetValue(name).ToString()
	return value
}

func (c *Collector) GetInt(name string) int {
	value, _ := c.GetValue(name).ToInteger()
	return int(value)
}

func (c *Collector) GetArrayOfString(name string) []string {
	export, _ := c.GetValue(name).Export()
	return export.([]string)
}

func (c *Collector) GetArrayOfArrayOfString(name string) [][]string {
	export, _ := c.GetValue(name).Export()
	return export.([][]string)
}

func (c *Collector) GetArrayOfArrayOfInterface(name string) [][]interface{} {
	export, _ := c.GetValue(name).Export()
	return export.([][]interface{})
}

func (c *Collector) Collect() (*SystemData, *[]PortData, error) {
	vm = otto.New()
	var loop_status []string
	var portstatus []string
	var speed []string
	var stats [][]interface{}
	var vlans [][]string

	// Login
	err := c.Login()
	if err != nil {
		return nil, nil, err
	}

	// Fetch and parse the javascript files containing all the data.
	for _, file := range []string{
		"system_data.js",
		"link_data.js",
		"VLAN_1Q_List_data.js",
	} {
		js, err := c.FetchJS(file)
		if err != nil {
			return nil, nil, err
		}
		if err = c.ParseJS(js); err != nil {
			return nil, nil, err
		}
	}
	systemData = SystemData{
		Max_port:    int64(c.GetInt("Max_port")),
		model_name:  c.GetString("model_name"),
		sys_fmw_ver: c.GetString("sys_fmw_ver"),
		sys_IP:      c.GetString("sys_IP"),
		sys_MAC:     c.GetString("sys_MAC"),
		loop:        c.GetString("loop"),
	}
	loop_status = c.GetArrayOfString("loop_status")
	portstatus = c.GetArrayOfString("portstatus")
	speed = c.GetArrayOfString("speed")
	stats = c.GetArrayOfArrayOfInterface("Stats")
	vlans = c.GetArrayOfArrayOfString("qvlans")

	// Clear the session.
	c.Logout()

	// Report number of configured vlans.
	for _, vlan := range vlans {
		systemData.vlans = append(systemData.vlans, vlan[0])
	}

	// Loop over ports
	portData = make([]PortData, systemData.Max_port)
	for i := range portData {
		portData[i].name = "port " + strconv.Itoa(i+1)
		portData[i].loop_status = loop_status[i]
		portData[i].portstatus = portstatus[i]
		portData[i].pvlan = "0"
		// Loop over configured vlans.
		for _, vlan := range vlans {
			// Parse vlan flags
			flag1, _ := strconv.ParseInt(strings.Replace(vlan[1], "0x", "", -1), 16, 64)
			if (flag1>>(i))&1 > 0 {
				// Current vlan is connected to current port.
				flag2, _ := strconv.ParseInt(strings.Replace(vlan[2], "0x", "", -1), 16, 64)
				if (flag2>>(i))&1 > 0 {
					// Tagged
					portData[i].vlans = append(portData[i].vlans, vlan[0])
				} else {
					// Untagged
					portData[i].pvlan = vlan[0]
				}
			}
		}

		// Port speed seems to always be "[num] [unit] [duplex]". Older versions
		// did not show duplex status.
		speedInfo := strings.Fields(speed[i])
		portData[i].speed, _ = strconv.Atoi(speedInfo[0])
		portData[i].speedUnit = speedInfo[1]
		if len(speedInfo) > 2 {
			portData[i].duplex = speedInfo[2]
		} else {
			portData[i].duplex = ""
		}

		// Sent/received traffic has a weird structure. This is what Zyxel's
		// code does:
		//
		//   tx = Stats[k][1]+Stats[k][2]+Stats[k][3];
		//   rx = Stats[k][6]+Stats[k][7]+Stats[k][8]+Stats[k][10];
		//   tx = parseFloat(tx).toLocaleString(); //Divide the numbers with commas
		//   rx = parseFloat(rx).toLocaleString();
		portData[i].stats.tx = float64(stats[i][1].(int64) + stats[i][2].(int64) + stats[i][3].(int64))
		portData[i].stats.rx = float64(stats[i][6].(int64) + stats[i][7].(int64) + stats[i][8].(int64) + stats[i][10].(int64))
	}

	return &systemData, &portData, nil
}

func (c *Collector) FetchJS(filename string) (string, error) {
	fileUrl := "http://" + c.address + "/" + filename
	log.Debug("Fetch " + fileUrl)
	resp, err := client.Get(fileUrl)
	if err != nil {
		log.Debug("... fetch error: ", err)
		c.Logout()
		return "", err
	}
	if resp.StatusCode != 200 {
		err := errors.New(resp.Status)
		log.Debug("... fetch error: ", err)
		c.Logout()
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Debug("... fetch error: ", err)
		c.Logout()
		return "", err
	}

	return string(body), nil
}

func (c *Collector) ParseJS(js string) error {
	log.Debug("Parse JavaScript\n" + js)
	_, err := vm.Run(js)
	if err != nil {
		log.Debug("... parse error: ", err)
		c.Logout()
		return err
	}
	return nil
}

func (c *Collector) EncryptPassword(password string) string {
	const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := ""
	for i := 0; i <= len(password); i++ {
		result = result + string(letters[rand.Intn(len(letters))])
		if i < len(password) {
			result = result + string(rune(int(password[i])-len(password)))
		}
	}
	return result
}

func (c *Collector) Login() error {
	// Log in on the GS1200.

	loginUrl := "http://" + c.address + "/login.cgi"
	log.Debug("Logging in at " + loginUrl)
	resp, err := client.PostForm(loginUrl, url.Values{"password": {c.EncryptPassword(c.password)}})
	if err != nil {
		log.Debug("... login error: ", err)
		// Even though logging in failed, try to log out, clearing the
		// session.
		c.Logout()
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Debug("... login error: ", err)
		return errors.New(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Debug("... login read error: ", err)
		c.Logout()
		return err
	}

	// Somebody else is logged in.
	if strings.Contains(string(body), "If a user is logged in already") {
		log.Debug("... login failed, already logged in")
		return errors.New("Logged in elsewhere")
	}

	// Curiously, a failed login wil happily return 200.
	if strings.Contains(string(body), "<title>Message</title>") &&
		strings.Contains(string(body), "alert(\"Incorrect password, please try again.\");") {
		log.Debug("... incorrect password")
		c.Logout()
		return errors.New("Incorrect password")
	}

	return nil
}

func (c *Collector) Logout() {
	logoutUrl := "http://" + c.address + "/logout.html"
	log.Debug("Logging out at " + logoutUrl)
	resp, err := client.Get(logoutUrl)
	if err != nil {
		log.Warn(err)
		return
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		log.Warn(err)
	}
}
