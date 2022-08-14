package repositories

import (
	"database/sql"
	"log"
	"superpose-sync/adapters/sqlite"
	"superpose-sync/utils"
)

//var workTreeRepository_CTE = `with cte as (
//	select id, name, is_dir, full_path, 1 as lev
//	from worktree
//	where parent is null
//	union all
//	select
//		w2.id,
//		(CASE cte.name
//           WHEN '{ROOT_DIR}'
//           THEN '{ROOT_DIR}'
//           ELSE cte.name||'/'
//       	END) ||w2.name,
//		w2.is_dir, w2.full_path, lev + 1
//	from cte
//	join worktree w2 on w2.parent = cte.id
//)`

type Path struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MimeType  string `json:"mime_type"`
	ChangedAt string `json:"changed_at"`
	CreatedAt string `json:"created_at"`
	IsDir     int    `json:"is_dir"`
	ParentID  string `json:"parent"`
	FullPath  string `json:"full_path"`
}

func (p Path) String() string {
	isDir := "0"
	if p.IsDir == 1 {
		isDir = "1"
	}

	str := "Path{\n"
	str += "  ID: " + p.ID + ",\n"
	str += "  Name: " + p.Name + ",\n"
	str += "  MimeType: " + p.MimeType + ",\n"
	str += "  ChangedAt: " + p.ChangedAt + ",\n"
	str += "  CreatedAt: " + p.CreatedAt + ",\n"
	str += "  IsDir: " + isDir + ",\n"
	str += "  ParentID: " + p.ParentID + ",\n"
	str += "  FullPath: " + p.FullPath + "\n"
	str += "}"
	return str
}

func Delete(path Path) error {
	query := "DELETE FROM worktree WHERE id = ?"
	stmt, err := sqlite.DB.Prepare(query)
	if err != nil {
		log.Println("Delete prepare error: ", err)
		return err
	}

	_, err = stmt.Exec(path.ID)
	if err != nil {
		log.Println("Delete execute error: ", err)
		return err
	}
	return nil
}

func Upsert(path Path) error {
	query := "INSERT INTO worktree (id, name, mime_type, created_at, changed_at, is_dir, parent, full_path) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?)" +
		"ON CONFLICT(id) DO UPDATE SET " +
		"name = excluded.name," +
		"mime_type = excluded.mime_type," +
		"created_at = excluded.created_at," +
		"changed_at = excluded.changed_at," +
		"is_dir = excluded.is_dir," +
		"parent = excluded.parent," +
		"full_path = excluded.full_path;"
	stmt, err := sqlite.DB.Prepare(query)
	//defer stmt.Close()
	if err != nil {
		log.Println("Upsert prepare error: ", err)
		return err
	}

	_, err = stmt.Exec(path.ID, path.Name, path.MimeType, path.CreatedAt, path.ChangedAt, path.IsDir, path.ParentID, path.FullPath)
	if err != nil {
		log.Println("Upsert execute error: ", err)
		return err
	}
	return nil
}

func GetIdByPath(path string) (string, error) {
	path = utils.GetAbsPathLocal(path)
	query := "select * from worktree where full_path = ?;"

	result := sqlite.DB.QueryRow(query, path)
	pathResult, err := hidratePath(result)
	if err != nil {
		return "", err
	}

	return pathResult.ID, err
}

func hidratePath(result *sql.Row) (Path, error) {
	pathResult := Path{}
	err := result.Scan(
		&pathResult.ID, &pathResult.Name, &pathResult.MimeType, &pathResult.CreatedAt, &pathResult.ChangedAt, &pathResult.IsDir,
		&pathResult.ParentID, &pathResult.FullPath,
	)
	return pathResult, err
}
