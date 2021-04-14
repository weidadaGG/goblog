package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"database/sql"

	"github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
)

var router = mux.NewRouter()
var db *sql.DB
func initDB() {
    var err error
    config := mysql.Config{
        User : "root",
        Passwd: "",
        Addr: "127.0.0.1:3306",
        Net: "tcp",
        DBName: "goblog",
        AllowNativePasswords: true,
    }
    //连接数据库
    db, err = sql.Open("mysql", config.FormatDSN())
    checkError(err)

    db.SetMaxOpenConns(25) //最大连接数
    db.SetMaxIdleConns(25) //最大空闲数
    db.SetConnMaxLifetime(5 * time.Minute)//每个连接的过期时间

    err = db.Ping()
    checkError(err)
}

func createTables() {
    createArticlesSQL := `CREATE TABLE IF NOT EXISTS articles(
    id bigint(20) PRIMARY KEY AUTO_INCREMENT NOT NULL,
    title varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
    body longtext COLLATE utf8mb4_unicode_ci
    );`
    _, err := db.Exec(createArticlesSQL)
    checkError(err)
}

func checkError(err error) {
    if err != nil {
        log.Fatal(err)
    }
}

func init() {
    initDB()
    createTables()
}


type Article struct{
    Title, Body string
    ID int64
}

type ArticlesFormData struct {
    Title, Body string
    URL *url.URL
    Errors map[string]string
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprint(w, "<h1>Hello, 欢迎来到 goblog！</h1>")
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprint(w, "此博客是用以记录编程笔记，如您有反馈或建议，请联系 "+
        "<a href=\"mailto:summer@example.com\">summer@example.com</a>")
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusNotFound)
    fmt.Fprint(w, "<h1>请求页面未找到 :(</h1><p>如有疑惑，请联系我们。</p>")
}


func articlesShowHandler(w http.ResponseWriter, r *http.Request) {
    id := getRouterParam("id", r)
    article, err := getArtilceByID(id)

    if err != nil {
        if err == sql.ErrNoRows {
            w.WriteHeader(http.StatusNotFound)
            fmt.Fprint(w, "404 文章未找到")
        } else {
            // 3.2 数据库错误
            checkError(err)
            w.WriteHeader(http.StatusInternalServerError)
            fmt.Fprint(w, "500 服务器内部错误")
        }
    } else {
        // tmpl, err := template.ParseFiles("resources/views/articles/show.gohtml")
        tmpl, err := template.New("show.gohtml").Funcs(template.FuncMap{
            "RouteName2URL": RouteName2URL,
            "Int64ToString": Int64ToString,
        }).ParseFiles("resources/views/articles/show.gohtml")
        checkError(err)
        tmpl.Execute(w, article)
    }
}

// RouteName2URL 通过路由名称来获取 URL
func RouteName2URL(routerName string, pars ...string) string {
    url, err := router.Get(routerName).URL(pars...)
    if err != nil {
        checkError(err)
        return ""
    }
    return url.String()
}

func Int64ToString(num int64) string {
    return strconv.FormatInt(num, 10)
}

func articlesIndexHandler(w http.ResponseWriter, r *http.Request) {
    // fmt.Fprint(w, "访问文章列表")
    rows, err := db.Query("SELECT * from articles")
    checkError(err)
    defer rows.Close()

    var articles []Article

    // 遍历数据
    for rows.Next() {
        var article Article
        // 扫描每一行的结果并赋值到一个 article 对象中
        err := rows.Scan(&article.ID, &article.Title, &article.Body)
        checkError(err)
        articles = append(articles, article)
    }
    // 检查遍历的时候是否发生错误
    err = rows.Err()
    checkError(err)

    tmpl, err := template.ParseFiles("resources/views/articles/index.gohtml")
    checkError(err)
    tmpl.Execute(w, articles)
}


func articlesStoreHandler(w http.ResponseWriter, r *http.Request) {
    title := r.PostFormValue("title")
    body := r.FormValue("body")
    errors := validateArticleFormData(title, body)

    if len(errors) == 0 {
        lastInsterID, err := saveArticleToDB(title, body)
       if lastInsterID > 0 {
            fmt.Fprint(w, "新增成功，ID：" + strconv.FormatInt(lastInsterID, 10))
       } else {
            checkError(err)
            w.WriteHeader(http.StatusInternalServerError)
            fmt.Fprint(w, "500 服务器错误")
       }
    } else {
        storeURL, _ := router.Get("articles.store").URL()
        data := ArticlesFormData{
            Title:  title,
            Body:   body,
            URL:    storeURL,
            Errors: errors,
        }
        // tmpl, err := template.New("create-form").Parse(html)
        tmpl, err := template.ParseFiles("resources/views/articles/edit.gohtml")
        if err != nil {
            panic(err)
        }

        tmpl.Execute(w, data)
    }

}

func saveArticleToDB(title string, body string) (int64, error) {

    // 变量初始化
    var (
        id   int64
        err  error
        rs   sql.Result
        stmt *sql.Stmt
    )

    // 1. 获取一个 prepare 声明语句
    stmt, err = db.Prepare("INSERT INTO articles (title, body) VALUES(?,?)")
    // 例行的错误检测
    if err != nil {
        return 0, err
    }

    // 2. 在此函数运行结束后关闭此语句，防止占用 SQL 连接
    defer stmt.Close()

    // 3. 执行请求，传参进入绑定的内容
    rs, err = stmt.Exec(title, body)
    if err != nil {
        return 0, err
    }

    // 4. 插入成功的话，会返回自增 ID
    if id, err = rs.LastInsertId(); id > 0 {
        return id, nil
    }

    return 0, err
}

func articlesAddHandler(w http.ResponseWriter, r *http.Request) {
    storeURL, _ := router.Get("articles.store").URL()
    tmpl, err := template.ParseFiles("resources/views/articles/edit.gohtml")
    if err != nil {
        panic(err)
    }
    data := ArticlesFormData{
        Title: "",
        Body: "",
        URL: storeURL,
        Errors: nil,
    }
    tmpl.Execute(w, data)
   
}

func articlesEditHandler(w http.ResponseWriter, r *http.Request) {
    id := getRouterParam("id", r)
    article, err := getArtilceByID(id)

    if err != nil {
        if err == sql.ErrNoRows {
            w.WriteHeader(http.StatusNotFound)
            fmt.Fprint(w, "404 文章未找到")
        } else {
            checkError(err)
            w.WriteHeader(http.StatusInternalServerError)
            fmt.Fprint(w, "500 服务器错误")
        }
    } else {
        updateUrl, _ := router.Get("articles.update").URL("id", id)
        data := ArticlesFormData{
            Title: article.Title,
            Body: article.Body,
            URL: updateUrl,
            Errors: nil,
        }
        tmpl, err := template.ParseFiles("resources/views/articles/edit.gohtml")
        checkError(err)
        tmpl.Execute(w, data)
    }
}

func articlesUpdateHandler(w http.ResponseWriter, r *http.Request) {
    // 1. 获取 URL 参数
    id := getRouterParam("id", r)

    // 2. 读取对应的文章数据
    _, err := getArtilceByID(id)

    // 3. 如果出现错误
    if err != nil {
        if err == sql.ErrNoRows {
            // 3.1 数据未找到
            w.WriteHeader(http.StatusNotFound)
            fmt.Fprint(w, "404 文章未找到")
        } else {
            // 3.2 数据库错误
            checkError(err)
            w.WriteHeader(http.StatusInternalServerError)
            fmt.Fprint(w, "500 服务器内部错误")
        }
    } else {
        // 4. 未出现错误

        // 4.1 表单验证
        title := r.PostFormValue("title")
        body := r.PostFormValue("body")
        errors := validateArticleFormData(title, body)

        if len(errors) == 0 {

            // 4.2 表单验证通过，更新数据
            query := "UPDATE articles SET title = ?, body = ? WHERE id = ?"
            rs, err := db.Exec(query, title, body, id)

            if err != nil {
                checkError(err)
                w.WriteHeader(http.StatusInternalServerError)
                fmt.Fprint(w, "500 服务器内部错误")
            }

            // √ 更新成功，跳转到文章详情页
            if n, _ := rs.RowsAffected(); n > 0 {
                showURL, _ := router.Get("articles.show").URL("id", id)
                http.Redirect(w, r, showURL.String(), http.StatusFound)
            } else {
                fmt.Fprint(w, "您没有做任何更改！")
            }
        } else {

            // 4.3 表单验证不通过，显示理由

            updateURL, _ := router.Get("articles.update").URL("id", id)
            data := ArticlesFormData{
                Title:  title,
                Body:   body,
                URL:    updateURL,
                Errors: errors,
            }
            tmpl, err := template.ParseFiles("resources/views/articles/edit.gohtml")
            checkError(err)

            tmpl.Execute(w, data)
        }
    }
}

func articlesDeleteHandler(w http.ResponseWriter, r *http.Request) {
    // 1. 获取 URL 参数
    id := getRouterParam("id", r)

    // 2. 读取对应的文章数据
    article, err := getArtilceByID(id)

    // 3. 如果出现错误
    if err != nil {
        if err == sql.ErrNoRows {
            // 3.1 数据未找到
            w.WriteHeader(http.StatusNotFound)
            fmt.Fprint(w, "404 文章未找到")
        } else {
            // 3.2 数据库错误
            checkError(err)
            w.WriteHeader(http.StatusInternalServerError)
            fmt.Fprint(w, "500 服务器内部错误")
        }
    } else {
        // 4. 未出现错误，执行删除操作
        rowsAffected, err := article.Delete()

        // 4.1 发生错误
        if err != nil {
            // 应该是 SQL 报错了
            checkError(err)
            w.WriteHeader(http.StatusInternalServerError)
            fmt.Fprint(w, "500 服务器内部错误")
        } else {
            // 4.2 未发生错误
            if rowsAffected > 0 {
                // 重定向到文章列表页
                indexURL, _ := router.Get("articles.index").URL()
                http.Redirect(w, r, indexURL.String(), http.StatusFound)
            } else {
                // Edge case
                w.WriteHeader(http.StatusNotFound)
                fmt.Fprint(w, "404 文章未找到")
            }
        }
    }
}

func (t Article) Delete() (rowsAffected int64, err error) {
    rs, err := db.Exec("DELETE FROM articles WHERE id = " + strconv.FormatInt(t.ID, 10))

    if err != nil {
        return 0, err
    }

    // √ 删除成功，跳转到文章详情页
    if n, _ := rs.RowsAffected(); n > 0 {
        return n, nil
    }

    return 0, nil
}

func getRouterParam(parameterName string, r *http.Request) string {
    vars := mux.Vars(r)
    return vars[parameterName]
}

func getArtilceByID(id string) (Article, error){
    article := Article{}
    query := "SELECT * FROM articles WHERE id = ?"
    err := db.QueryRow(query, id).Scan(&article.ID, &article.Title, &article.Body)
    return article, err
}

func validateArticleFormData(title string, body string) map[string]string {
    errors := make(map[string]string)
    
    // 验证标题
    if title == "" {
        errors["title"] = "标题不能为空"
    } else if utf8.RuneCountInString(title) < 3 || utf8.RuneCountInString(title) > 40 {
        errors["title"] = "标题长度需介于 3-40"
    }

    // 验证内容
    if body == "" {
        errors["body"] = "内容不能为空"
    } else if utf8.RuneCountInString(body) < 10 {
        errors["body"] = "内容长度需大于或等于 10 个字节"
    }

    return errors
}

func (t Article) Link() string {
    showUrl, err := router.Get("articles.show").URL("id", strconv.FormatInt(t.ID, 10))
    if err != nil {
        checkError(err)
        return ""
    }
    return showUrl.String()
}

func foreaHtmlMiddlewaer (next http.Handler) http.Handler {
    return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request)  {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        next.ServeHTTP(w, r)
    })
}

func removeTrailingSlash (next http.Handler) http.Handler {
    return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request)  {
        if r.URL.Path != "/" {
            r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
        }
        next.ServeHTTP(w, r)
    })
}

func main() {
    router.HandleFunc("/", homeHandler).Methods("GET").Name("home")
    router.HandleFunc("/about", aboutHandler).Methods("GET").Name("about")

    router.HandleFunc("/articles/{id:[0-9]+}", articlesShowHandler).Methods("GET").Name("articles.show")
    router.HandleFunc("/articles", articlesIndexHandler).Methods("GET").Name("articles.index")
    router.HandleFunc("/articles", articlesStoreHandler).Methods("POST").Name("articles.store")
    router.HandleFunc("/articles/add", articlesAddHandler).Methods("GET").Name("articles.add")
    router.HandleFunc("/articles/{id:[0-9]+}/edit", articlesEditHandler).Methods("GET").Name("articles.edit")
    router.HandleFunc("/articles/{id:[0-9]+}", articlesUpdateHandler).Methods("POST").Name("articles.update")
    router.HandleFunc("/articles/{id:[0-9]+}/delete", articlesDeleteHandler).Methods("POST").Name("articles.delete")

    // 自定义 404 页面
    router.NotFoundHandler = http.HandlerFunc(notFoundHandler)

    router.Use(foreaHtmlMiddlewaer)

    // 通过命名路由获取 URL 示例
    // homeURL, _ := router.Get("home").URL()
    // fmt.Println("homeURL: ", homeURL)
    // articleURL, _ := router.Get("articles.show").URL("id", "23")
    // fmt.Println("articleURL: ", articleURL)

    http.ListenAndServe(":3000", removeTrailingSlash(router))
}