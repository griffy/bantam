package bantam

import (
    "container/list"
    "regexp"
    "strconv"
    "strings"
	"github.com/hoisie/web.go"
    "github.com/hoisie/mustache.go"
    "github.com/stathat/jconfig"
    "launchpad.net/goforms"
    "github.com/griffy/birdbrain"
    "github.com/griffy/birdbrain/store"
    "github.com/griffy/htmlfiller"
)

var sessionStore birdbrain.SessionStore

var paramNameRegex *regexp.Regexp

func init() {
    // FIXME: should use lazy qualifier when Go
    //        has more advanced regexp package
    paramNameRegex = regexp.MustCompile("\\{.*\\}")
}

type view interface {
    Uri() string
}

type viewHelper struct {
    view // anonymous field so all its fields are still accessible
    Flash bool
    FlashType, FlashMessage string
}

type Conn struct {
    *web.Context
    RouteParams map[string]string
    Session *birdbrain.Session
    Tmpl map[string]interface{}
}

func deflattenParams(params map[string]string) map[string][]string {
    fullParams := make(map[string][]string)
    for name, val := range params {
        fullParams[name] = []string{val}
    }
    return fullParams
}

func (c *Conn) ValidParams(form *forms.Form) bool {
    // TODO: add a 'background' xsrf check here
    form.SetFormData(c.FullParams)
    // calling IsValid results in form's CleanedData being
    // populated if it passes validation, otherwise
    // form's errors are populated
    return form.IsValid()
}

func (c *Conn) ValidRouteParams(form *forms.Form) bool {
    fullParams := deflattenParams(c.RouteParams)
    form.SetFormData(fullParams)
    return form.IsValid()
}

func (c *Conn) getFlash() (string, string) {
    flash, err := c.Session.Get("_flash")
    if err == nil {
        c.Session.Delete("_flash")
        flashPieces := strings.SplitN(flash, ":", 2)
        return flashPieces[0], flashPieces[1]
    }
    return "", ""
}

func (c *Conn) addTemplateFlash() {
    flashType, flashMsg := c.getFlash()
    c.Tmpl["Flash"] = (flashMsg != "")
    c.Tmpl["FlashType"] = flashType
    c.Tmpl["FlashMessage"] = flashMsg
}

func (c *Conn) addViewFlash(v view) *viewHelper {
    flashType, flashMsg := c.getFlash()
    return &viewHelper{v, (flashMsg != ""), flashType, flashMsg}
}

func (c *Conn) Render(file interface{}, forms ...*forms.Form) {
    // TODO: c.Tmpl["h"] = &helpers.Helpers{}
    res := ""
    switch (file.(type)) {
    case string:
        uri, _ := file.(string)
        c.addTemplateFlash()
        res = mustache.RenderFile(uri, c.Tmpl)
    case view:
        v, _ := file.(view)
        view := c.addViewFlash(v)
        res = mustache.RenderFile(view.Uri(), view)
    }
    // if there are forms supplied, the user failed
    // at providing valid input, so we must fill in the
    // appropriate form values along with the error messages
    for _, form := range forms {
        defaultVals := make(map[string]string)
        for name, field := range form.Fields {
            defaultVals[name] = field.Value()
        }
        res = htmlfiller.Fill(res, defaultVals, form.Errors)
    }
    c.WriteString(res)
}

func (c *Conn) Redirect(addr string, args ...int) {
    if len(args) > 0 {
        c.Context.Redirect(args[0], addr)
    } else {
        c.Context.Redirect(302, addr)
    }
}

func (c *Conn) Flash(typeof, message string) {
    c.Session.Set("_flash", typeof + ":" + message)    
}

type RouteHandler func(*Conn)

func buildHandler(handler RouteHandler, paramNames []string) func(*web.Context, ...string) {
    // web.go looks to see if the first parameter to a
    // handler function is a Context object before deciding
    // to pass it, so we must do so
    return func(ctx *web.Context, params ...string) {
        // use the context to either start a new session or resume
        // an existing one
        session := birdbrain.NewSession(ctx, sessionStore)
        // create a Conn object that holds context, params from
        // the route/url, session, and a 
        // map of keys to values that will be passed to the 
        // page template for rendering
        paramMap := make(map[string]string)
        for i, p := range paramNames {
            paramMap[p] = params[i]
        }
        c := &Conn{ctx, paramMap, session, make(map[string]interface{})}
        handler(c)
    }
}

func removeTrailingSlash(uri string) string {
    if strings.HasSuffix(uri, "/") {
        uri = uri[:len(uri)-1]
    }
    return uri
}

func extractParamNames(route string) []string {
    names := []string{}
    queue := list.New()
    useQueue := false
    for _, char := range route {
        if char == '{' {
            useQueue = true
            queue = list.New()
        } else if char == '}' {
            useQueue = false
            paramName := ""
            for e := queue.Back(); e != nil; e = e.Prev() {
                codePoint, _ := e.Value.(int)
                paramName += string([]int{codePoint})
            }
            names = append(names, paramName)
        } else {
            if useQueue {
                queue.PushFront(char)
            }
        }
    }
    return names
}

func toRegex(route string) (regex string) {
    // FIXME: should use lazy qualifier when Go
    //        has more advanced regexp package
    regex = paramNameRegex.ReplaceAllString(route, "(.*)")
    // make the final slash optional, directing to the same route
    // FIXME: should we instead issue redirects rather than
    //        allowing two urls to point to the same resource?
    regex += "/?"
    return
}

func prepareRoute(route string) (string, []string) {
    route = removeTrailingSlash(route)
    paramNames := extractParamNames(route)
    regexRoute := toRegex(route)
    return regexRoute, paramNames
}

func Get(route string, handler RouteHandler) {
    regexRoute, paramNames := prepareRoute(route)
    web.Get(regexRoute, buildHandler(handler, paramNames))
}

func Post(route string, handler RouteHandler) {
    regexRoute, paramNames := prepareRoute(route)
    web.Post(regexRoute, buildHandler(handler, paramNames))
}

func Put(route string, handler RouteHandler) {
    regexRoute, paramNames := prepareRoute(route)
    web.Put(regexRoute, buildHandler(handler, paramNames))
}

func Delete(route string, handler RouteHandler) {
    regexRoute, paramNames := prepareRoute(route)
    web.Delete(regexRoute, buildHandler(handler, paramNames))
}

func Run() {
    // TODO: set up logger and config here
    //InitLogger()
    //InitMemcache()
    //web.SetLogger(Logger)
    // TODO: add error checking
    config := jconfig.LoadConfig("app.conf")
    web.Config.StaticDir = config.GetString("StaticDirectory")
    web.Config.Addr = config.GetString("Host")
    web.Config.Port = config.GetInt("Port")
    web.Config.CookieSecret = config.GetString("CookieSecret")
    switch config.GetString("SessionStore") {
    case "redis":
        host := config.GetString("SessionHost")
        port := strconv.Itoa(config.GetInt("SessionPort"))
        sessionStore = store.NewRedisStore("tcp:" + host + ":" + port)
    }
    addr := web.Config.Addr + ":" + strconv.Itoa(web.Config.Port)
    web.Run(addr)
}
