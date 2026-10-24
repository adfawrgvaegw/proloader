package constants


// CommitInfo 提交信息结构体
type CommitInfo struct {
	Hash        string `json:"hash"`
	Author      string `json:"author"`
	Date        string `json:"date"`
	Description string `json:"description"`
}


var CommitLog []CommitInfo


// GetCommitsSince 获取指定提交次数之后的提交
func GetCommitsSince(lastCommitNum int) []CommitInfo {
	if lastCommitNum >= len(CommitLog) || lastCommitNum < 0 {
		return []CommitInfo{}
	}

	// 返回从指定位置到最新的提交（git log是按时间倒序的）
	newCommitsCount := len(CommitLog) - lastCommitNum
	if newCommitsCount <= 0 {
		return []CommitInfo{}
	}

	return CommitLog[:newCommitsCount]
}
