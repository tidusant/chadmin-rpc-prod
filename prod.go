package main

import (
	"github.com/tidusant/c3m-common/c3mcommon"
	"github.com/tidusant/c3m-common/inflect"
	"github.com/tidusant/c3m-common/log"
	"github.com/tidusant/c3m-common/lzjs"
	"github.com/tidusant/c3m-common/mystring"
	rpb "github.com/tidusant/chadmin-repo/builder"
	rpch "github.com/tidusant/chadmin-repo/cuahang"
	"github.com/tidusant/chadmin-repo/models"

	//	"c3m/common/inflect"
	//	"c3m/log"
	"encoding/json"
	"flag"
	"net"
	"net/rpc"
	"strconv"
	"strings"
	"time"
)

const (
	defaultcampaigncode string = "XVsdAZGVmYd"
)

type Arith int

func (t *Arith) Run(data string, result *string) error {
	log.Debugf("Call RPCprod args:" + data)
	*result = ""
	//parse args
	args := strings.Split(data, "|")

	if len(args) < 3 {
		return nil
	}
	var usex models.UserSession
	usex.Session = args[0]
	usex.Action = args[2]
	info := strings.Split(args[1], "[+]")
	usex.UserID = info[0]
	ShopID := info[1]
	usex.Params = ""
	if len(args) > 3 {
		usex.Params = args[3]
	}

	//check shop permission
	shop := rpch.GetShopById(usex.UserID, ShopID)
	if shop.Status == 0 {
		*result = c3mcommon.ReturnJsonMessage("-4", "Shop is disabled.", "", "")
		return nil
	}
	usex.Shop = shop

	if usex.Action == "l" {
		*result = LoadProduct(usex, true)

	} else if usex.Action == "ls" {
		*result = LoadProduct(usex, false)

	} else if usex.Action == "lc" {
		*result = LoadCat(usex)
	} else if usex.Action == "ld" {
		*result = LoadDetail(usex)
	} else if usex.Action == "sc" {
		*result = SaveCat(usex)
	} else if usex.Action == "rc" {
		*result = RemoveCat(usex)
	} else if usex.Action == "s" {
		*result = SaveProduct(usex)
	} else if usex.Action == "r" {
		*result = RemoveProduct(usex)
	} else { //default
		*result = c3mcommon.ReturnJsonMessage("-5", "Action not found.", "", "")
	}

	return nil
}
func LoadCat(usex models.UserSession) string {
	log.Debugf("loadcat begin")
	cats := rpch.GetAllCats(usex.UserID, usex.Shop.ID.Hex())
	strrt := "["
	catinfstr := ""
	for _, cat := range cats {
		catlangs := ""
		for lang, catinf := range cat.Langs {
			info, _ := json.Marshal(catinf)
			catlangs += "\"" + lang + "\":" + string(info) + ","
		}
		catlangs = catlangs[:len(catlangs)-1]
		catinfstr += "{\"Code\":\"" + cat.Code + "\",\"Langs\":{" + catlangs + "}},"
	}
	if catinfstr == "" {
		strrt += "{}]"
	} else {
		strrt += catinfstr[:len(catinfstr)-1] + "]"
	}
	log.Debugf("loadcat %s", strrt)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func LoadDetail(usex models.UserSession) string {
	prod := rpch.GetProdByCode(usex.Shop.ID.Hex(), usex.Params)
	info, _ := json.Marshal(prod)
	strrt := string(info)
	log.Debugf("loadcat %s", strrt)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func SaveCat(usex models.UserSession) string {
	var cat models.ProdCat
	err := json.Unmarshal([]byte(usex.Params), &cat)
	if !c3mcommon.CheckError("create cat parse json", err) {
		return c3mcommon.ReturnJsonMessage("0", "create catalog fail", "", "")
	}
	olditem := cat
	newcat := false
	if cat.Code == "" {
		newcat = true
	}

	//get all cats
	cats := rpch.GetAllCats(usex.UserID, usex.Shop.ID.Hex())
	//check max cat limited
	if newcat {
		shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
		if shop.Config.MaxCat <= len(cats) {
			return c3mcommon.ReturnJsonMessage("3", "error", "max limit reach", "")
		}
	}
	//get all slug
	slugs := rpch.GetAllSlug(usex.UserID, usex.Shop.ID.Hex())
	mapslugs := make(map[string]string)
	for i := 0; i < len(slugs); i++ {
		mapslugs[slugs[i]] = slugs[i]
	}
	//get array of code
	catcodes := make(map[string]string)
	//get old item
	for _, c := range cats {
		catcodes[c.Code] = c.Code
		if !newcat && c.Code == cat.Code {
			olditem = c
		}
	}

	for lang, _ := range cat.Langs {
		if cat.Langs[lang].Name == "" {
			delete(cat.Langs, lang)
			continue
		}
		//newslug
		tb, _ := lzjs.DecompressFromBase64(cat.Langs[lang].Name)
		newslug := inflect.Parameterize(string(tb))
		cat.Langs[lang].Slug = newslug

		isChangeSlug := true
		if !newcat {
			if olditem.Langs[lang].Slug == newslug {
				isChangeSlug = false
			}
		}

		if isChangeSlug {
			//check slug duplicate
			i := 1
			for {
				if _, ok := mapslugs[cat.Langs[lang].Slug]; ok {
					cat.Langs[lang].Slug = newslug + strconv.Itoa(i)
					i++
				} else {
					mapslugs[cat.Langs[lang].Slug] = cat.Langs[lang].Slug
					break
				}
			}
			//remove oldslug
			if !newcat {
				rpch.RemoveSlug(olditem.Langs[lang].Slug, usex.Shop.ID.Hex())
			}
			rpch.CreateSlug(cat.Langs[lang].Slug, usex.Shop.ID.Hex(), "prodcats")
		}
	}

	//check code duplicate
	if newcat {
		//insert new
		newcode := ""
		for {
			newcode = mystring.RandString(3)
			if _, ok := catcodes[newcode]; !ok {
				break
			}
		}
		cat.Code = newcode
		cat.ShopId = usex.Shop.ID.Hex()
		cat.UserId = usex.UserID
		cat.Created = time.Now().UTC().Add(time.Hour + 7)
	} else {
		//update
		olditem.Langs = cat.Langs
		cat = olditem
	}
	strrt := rpch.SaveCat(cat)
	if strrt == "0" {
		return c3mcommon.ReturnJsonMessage("0", "error", "error", "")
	}
	log.Debugf("saveprod %s", strrt)
	//build cat
	var bs models.BuildScript
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = shop.Theme
	bs.Domain = shop.Domain

	bs.Collection = "prodcats"
	bs.ObjectID = cat.Code
	rpb.CreateBuild(bs)

	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func RemoveCat(usex models.UserSession) string {
	log.Debugf("remove cat %s", usex.Params)
	args := strings.Split(usex.Params, ",")
	if len(args) < 2 {
		return c3mcommon.ReturnJsonMessage("0", "error submit fields", "", "")
	}
	log.Debugf("save prod %s", args)
	code := args[0]
	lang := args[1]
	//check product

	prods := rpch.GetProdsByCatId(usex.Shop.ID.Hex(), code)
	for _, prod := range prods {
		if prod.Langs[lang] != nil {
			return c3mcommon.ReturnJsonMessage("2", "Catalog not empty", "", "")
		}
	}
	cat := rpch.GetCatByCode(usex.Shop.ID.Hex(), code)
	if cat.Langs[lang] != nil {
		//remove slug
		rpch.RemoveSlug(cat.Langs[lang].Slug, usex.Shop.ID.Hex())
		delete(cat.Langs, lang)
		rpch.SaveCat(cat)
	}

	//build cat
	var bs models.BuildScript
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = shop.Theme
	bs.Domain = shop.Domain
	bs.Collection = "rmprodcats"
	bs.ObjectID = cat.Code
	rpb.CreateBuild(bs)

	return c3mcommon.ReturnJsonMessage("1", "", "success", "")

}
func RemoveProduct(usex models.UserSession) string {
	log.Debugf("remove prod %s", usex.Params)
	args := strings.Split(usex.Params, ",")
	if len(args) < 2 {
		return c3mcommon.ReturnJsonMessage("0", "error submit fields", "", "")
	}
	log.Debugf("save prod %s", args)
	code := args[0]
	lang := args[1]
	prod := rpch.GetProdByCode(usex.Shop.ID.Hex(), code)
	if prod.Langs[lang] != nil {
		//remove slug
		rpch.RemoveSlug(prod.Langs[lang].Slug, usex.Shop.ID.Hex())
		delete(prod.Langs, lang)
		rpch.SaveProd(prod)
	}

	//build cat
	var bs models.BuildScript
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = shop.Theme
	bs.Domain = shop.Domain
	bs.Collection = "prodcats"
	bs.ObjectID = prod.CatId
	rpb.CreateBuild(bs)

	//remove prod
	bs.Collection = "rmproduct"
	bs.ObjectID = prod.Code
	rpb.CreateBuild(bs)
	return c3mcommon.ReturnJsonMessage("1", "", "success", "")

}

func SaveProduct(usex models.UserSession) string {

	var prod models.Product
	err := json.Unmarshal([]byte(usex.Params), &prod)
	if !c3mcommon.CheckError("create prod parse json", err) {
		return c3mcommon.ReturnJsonMessage("0", "create prod fail", "", "")
	}

	prod.UserId = usex.UserID
	prod.ShopId = usex.Shop.ID.Hex()
	if prod.Code == "" {
		prod.Created = time.Now().UTC().Add(time.Hour + 7)
	}
	prod.Modified = time.Now().UTC().Add(time.Hour + 7)

	//get all product
	prods := rpch.GetAllProds(prod.UserId, prod.ShopId, true)
	newprod := false
	if prod.Code == "" {
		newprod = true
	}
	//check limit:
	if newprod {

		if usex.Shop.Config.MaxProd <= len(prods) {
			return c3mcommon.ReturnJsonMessage("3", "max prod limit", "error", "")
		}
	}

	prodcodes := make(map[string]string)
	propcodes := make(map[string]string)
	var olditem models.Product
	for _, item := range prods {
		prodcodes[item.Code] = item.Code
		for _, prop := range item.Properties {
			propcodes[prop.Code] = prop.Code
		}
		if !newprod && item.Code == prod.Code {
			olditem = item
		}
	}

	//check edit and oldprod
	if !newprod && olditem.Code == "" {
		return c3mcommon.ReturnJsonMessage("2", "not found", "error", "")
	}

	//slug
	//get all slug
	slugs := rpch.GetAllSlug(usex.UserID, usex.Shop.ID.Hex())
	mapslugs := make(map[string]string)
	for i := 0; i < len(slugs); i++ {
		mapslugs[slugs[i]] = slugs[i]
	}
	for lang, _ := range prod.Langs {
		if prod.Langs[lang].Name == "" {
			//check if oldprod has value, else delete
			if olditem.Langs[lang] == nil {
				delete(prod.Langs, lang)
			} else {
				//not update for null lang
				if prod.Langs[lang].Description != "" || prod.Langs[lang].Avatar != "" || prod.Langs[lang].BasePrice != 0 || prod.Langs[lang].DiscountPrice != 0 || len(prod.Langs[lang].Images) > 0 {
					prod.Langs[lang] = olditem.Langs[lang]
				} else {
					delete(prod.Langs, lang)
				}
			}
			continue
		}
		//newslug
		tb, _ := lzjs.DecompressFromBase64(prod.Langs[lang].Name)
		newslug := inflect.Parameterize(string(tb))
		prod.Langs[lang].Slug = newslug
		isChangeSlug := true
		if !newprod {
			if olditem.Langs[lang].Slug == newslug {
				isChangeSlug = false
			}
		}

		if isChangeSlug {
			//check slug duplicate
			i := 1
			for {
				if _, ok := mapslugs[prod.Langs[lang].Slug]; ok {
					prod.Langs[lang].Slug = newslug + strconv.Itoa(i)
					i++
				} else {
					mapslugs[prod.Langs[lang].Slug] = prod.Langs[lang].Slug
					break
				}
			}
			//remove oldslug
			if !newprod {
				rpch.RemoveSlug(olditem.Langs[lang].Slug, usex.Shop.ID.Hex())
			}
			rpch.CreateSlug(prod.Langs[lang].Slug, usex.Shop.ID.Hex(), "prodcats")
		}

		if prod.Langs[lang].Unit == "" {
			prod.Langs[lang].Unit = "unit"
		}
	}

	//create code
	if newprod {
		for {
			prod.Code = mystring.RandString(3)
			if _, ok := prodcodes[prod.Code]; !ok {
				break
			}
		}
		prod.Main = true
		prod.Publish = true
	} else {
		olditem.Langs = prod.Langs
		olditem.Properties = prod.Properties
		prod = olditem
	}

	//create prop code
	for k, prop := range prod.Properties {
		if strings.Trim(prop.Code, " ") == "" {
			for {
				prop.Code = mystring.RandString(4)
				if _, ok := propcodes[prop.Code]; !ok {
					propcodes[prop.Code] = prop.Code
					prod.Properties[k].Code = prop.Code
					break
				}
			}
		}
	}

	strrt := rpch.SaveProd(prod)
	if strrt == "0" {
		return c3mcommon.ReturnJsonMessage("0", "error", "error", "")
	}
	log.Debugf("saveprod %s", strrt)
	//build cat
	var bs models.BuildScript

	bs.ShopID = usex.Shop.ID.Hex()
	bs.TemplateCode = usex.Shop.Theme
	bs.Domain = usex.Shop.Domain
	bs.Collection = "prodcats"
	bs.ObjectID = prod.CatId
	rpb.CreateBuild(bs)

	//build cat
	bs.Collection = "product"
	bs.ObjectID = prod.Code
	rpb.CreateBuild(bs)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func LoadProduct(usex models.UserSession, isMain bool) string {

	prods := rpch.GetAllProds(usex.UserID, usex.Shop.ID.Hex(), true)
	if len(prods) == 0 {
		return c3mcommon.ReturnJsonMessage("2", "", "no prod found", "")
	}

	strrt := "["

	for _, prod := range prods {
		strlang := "{"
		for lang, langinfo := range prod.Langs {
			langinfo.Description = ""
			langinfo.Content = ""
			info, _ := json.Marshal(langinfo)
			strlang += "\"" + lang + "\":" + string(info) + ","
		}
		strlang = strlang[:len(strlang)-1] + "}"
		info, _ := json.Marshal(prod.Properties)
		props := string(info)
		strrt += "{\"Code\":\"" + prod.Code + "\",\"CatId\":\"" + prod.CatId + "\",\"Langs\":" + strlang + ",\"Properties\":" + props + "},"
	}
	strrt = strrt[:len(strrt)-1] + "]"
	log.Debugf("loadprod %s", strrt)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)

}
func main() {
	var port int
	var debug bool
	flag.IntVar(&port, "port", 9880, "help message for flagname")
	flag.BoolVar(&debug, "debug", false, "Indicates if debug messages should be printed in log files")
	flag.Parse()

	//logLevel := log.DebugLevel
	if !debug {
		//logLevel = log.InfoLevel

	}

	// log.SetOutputFile(fmt.Sprintf("adminDash-"+strconv.Itoa(port)), logLevel)
	// defer log.CloseOutputFile()
	// log.RedirectStdOut()

	//init db
	arith := new(Arith)
	rpc.Register(arith)
	log.Infof("running with port:" + strconv.Itoa(port))

	//			rpc.HandleHTTP()
	//			l, e := net.Listen("tcp", ":"+strconv.Itoa(port))
	//			if e != nil {
	//				log.Debug("listen error:", e)
	//			}
	//			http.Serve(l, nil)

	tcpAddr, err := net.ResolveTCPAddr("tcp", ":"+strconv.Itoa(port))
	c3mcommon.CheckError("rpc dail:", err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	c3mcommon.CheckError("rpc init listen", err)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go rpc.ServeConn(conn)
	}
}
