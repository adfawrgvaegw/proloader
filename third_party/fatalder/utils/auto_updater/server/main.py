from typing import Optional, Tuple, Dict
import os
import requests
from github import Github, Auth
import pygit2
from rich.console import Console
from rich.progress import Progress, BarColumn, DownloadColumn, TextColumn, TimeRemainingColumn, TransferSpeedColumn
from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse, PlainTextResponse
import asyncio
import zipfile
import hashlib
import uvicorn
import shutil

from constants import GITHUB_API_KEY, GITHUB_REPO_URL

console = Console()

def find_git_root(current_path=None):
    current_path = current_path or os.getcwd()
    while True:
        try:
            # 尝试打开当前路径下的 Git 仓库
            repo = pygit2.Repository(current_path)
            # workdir 为仓库的工作区根目录（不含 .git 文件夹）
            return repo.workdir
        except pygit2.GitError:
            # 若当前路径不是仓库，向上移动到父目录
            parent_path = os.path.dirname(current_path)
            if parent_path == current_path:  # 到达系统根目录
                return None
            current_path = parent_path

current_dir = os.path.dirname(os.path.abspath(__file__))  # 当前脚本所在目录
root_dir = find_git_root(current_dir)
git_repo = pygit2.Repository(root_dir)
last_commit = git_repo.revparse_single("HEAD")
last_commit_sha = str(last_commit.id)

def find_latest_success_artifact(
    repo,
    workflow_filename: str,
    artifact_name: str,
    commit_sha: Optional[str] = None,
    any_commit: bool = False,
) -> Tuple[Optional[object], Optional[object]]:
    """查找指定工作流的最新成功运行中的目标 artifact。

    Args:
        repo: GitHub 仓库对象
        workflow_filename: 工作流文件名
        artifact_name: artifact 名称
        commit_sha: 指定的提交 SHA（可选）
        any_commit: 是否允许任何提交，不限制特定提交（用于初始化）

    返回 (artifact, run)，若未找到则返回 (None, None)。
    """
    workflow = repo.get_workflow(workflow_filename)
    runs = workflow.get_runs(status="completed")
    for run in runs:
        # 如果指定了 commit_sha，只匹配该提交
        if commit_sha:
            if str(run.head_sha) != commit_sha:
                continue
        # 如果不允许任何提交，则按 last_commit_sha 过滤
        elif not any_commit:
            if str(run.head_sha) != last_commit_sha:
                continue
        # any_commit=True 时不进行提交过滤，查找任何成功的构建
        
        if getattr(run, "conclusion", None) != "success":
            continue
        artifacts = list(run.get_artifacts())
        if not artifacts:
            continue
        target = next((a for a in artifacts if a.name == artifact_name), None)
        if target is None:
            target = artifacts[0]
        return target, run
    return None, None


def download_with_progress(url: str, token: str, dst_path: str) -> str:
    """带进度条下载文件到 dst_path。支持 GitHub artifacts 302 跳转。"""
    headers = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "User-Agent": "FatalderAutoUpdater",
    }

    with requests.get(url, headers=headers, stream=True, allow_redirects=True) as resp:
        resp.raise_for_status()
        total = int(resp.headers.get("Content-Length", 0))
        os.makedirs(os.path.dirname(dst_path), exist_ok=True)

        progress_columns = [
            TextColumn("[bold blue]下载"),
            BarColumn(),
            DownloadColumn(),
            TransferSpeedColumn(),
            TimeRemainingColumn(),
        ]

        with open(dst_path, "wb") as fp, Progress(*progress_columns) as progress:
            task_id = progress.add_task("download", total=total if total > 0 else None)
            for chunk in resp.iter_content(chunk_size=256 * 1024):
                if not chunk:
                    continue
                fp.write(chunk)
                progress.update(task_id, advance=len(chunk))

    return dst_path


app = FastAPI()

APP_DIR = current_dir
REPO_ROOT = root_dir or os.getcwd()
DOWNLOADS_DIR = os.path.join(APP_DIR, "downloads")
EXTRACT_DIR = os.path.join(DOWNLOADS_DIR, "extracted")
os.makedirs(DOWNLOADS_DIR, exist_ok=True)
os.makedirs(EXTRACT_DIR, exist_ok=True)

class ArtifactIndex:
    def __init__(self, base_downloads_dir: str) -> None:
        self.base_downloads_dir = base_downloads_dir
        self.filename_to_path: Dict[str, str] = {}
        self.filename_to_sha512: Dict[str, str] = {}

    def clear(self) -> None:
        self.filename_to_path.clear()
        self.filename_to_sha512.clear()

    def index_dir(self, target_dir: str) -> None:
        self.clear()
        for root, _, files in os.walk(target_dir):
            for name in files:
                abs_p = os.path.join(root, name)
                key = os.path.basename(abs_p)
                try:
                    digest = sha512_of_file(abs_p)
                except Exception:
                    continue
                self.filename_to_path[key] = abs_p
                self.filename_to_sha512[key] = digest

INDEX = ArtifactIndex(DOWNLOADS_DIR)

def extract_zip(zip_path: str, output_dir: str) -> None:
    os.makedirs(output_dir, exist_ok=True)
    with zipfile.ZipFile(zip_path, 'r') as zf:
        zf.extractall(output_dir)

def sha512_of_file(file_path: str) -> str:
    h = hashlib.sha512()
    with open(file_path, 'rb') as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()

@app.get("/sha-512/{filename}", response_class=PlainTextResponse)
def get_sha512(filename: str):
    if "/" in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid filename")
    digest = INDEX.filename_to_sha512.get(filename)
    if not digest:
        # 提供更详细的调试信息
        available_files = list(INDEX.filename_to_sha512.keys())
        console.print(f"[red]请求文件 '{filename}' 未找到[/red]")
        console.print(f"[blue]可用文件: {available_files}[/blue]")
        raise HTTPException(status_code=404, detail="not found")
    return digest

@app.get("/downloads/{filename}")
def download_file(filename: str):
    if "/" in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid filename")
    path = INDEX.filename_to_path.get(filename)
    if not path or not os.path.isfile(path):
        raise HTTPException(status_code=404, detail="not found")
    return FileResponse(path, filename=filename)

def find_run_for_commit(repo, workflow_filename: str, commit_sha: str):
    workflow = repo.get_workflow(workflow_filename)
    runs = workflow.get_runs()
    for r in runs:
        if getattr(r, "head_sha", None) == commit_sha:
            return r
    return None

def clean_directory(dir_path: str) -> None:
    if not os.path.isdir(dir_path):
        return
    for root, dirs, files in os.walk(dir_path, topdown=False):
        for name in files:
            try:
                os.remove(os.path.join(root, name))
            except Exception:
                pass
        for name in dirs:
            shutil.rmtree(os.path.join(root, name), ignore_errors=True)

def extract_and_index(zip_path: str) -> None:
    # 始终只保留最新版本：清空提取目录，再解压并重建索引
    os.makedirs(EXTRACT_DIR, exist_ok=True)
    clean_directory(EXTRACT_DIR)
    extract_zip(zip_path, EXTRACT_DIR)
    INDEX.index_dir(EXTRACT_DIR)

async def poll_commit_loop():
    auth = Auth.Token(GITHUB_API_KEY)
    gh = Github(auth=auth)
    repo = gh.get_repo(GITHUB_REPO_URL)
    workflow_file = ".github/workflows/cli-build.yml"
    artifact_name = "Fatalder-cli-artifacts"

    last_seen_sha = last_commit_sha
    console.print(f"[yellow]初始提交: {last_seen_sha}[/yellow]")

    while True:
        try:
            repo_latest_sha = str(pygit2.Repository(REPO_ROOT).revparse_single("HEAD").id)
            if repo_latest_sha == last_seen_sha:
                await asyncio.sleep(1)
                continue
            console.print(f"[cyan]检测到新提交: {repo_latest_sha}[/cyan]")
            last_seen_sha = repo_latest_sha

            run = find_run_for_commit(repo, workflow_file, repo_latest_sha)
            attempts = 0
            while run is None and attempts < 10:
                await asyncio.sleep(30)
                run = find_run_for_commit(repo, workflow_file, repo_latest_sha)
                attempts += 1
            if run is None:
                console.print("[red]未找到该提交对应的 workflow run[/red]")
                continue

            while True:
                run = repo.get_workflow_run(run.id)
                status = getattr(run, "status", None)
                conclusion = getattr(run, "conclusion", None)
                console.print(f"[blue]run 状态: {status}, 结论: {conclusion}[/blue]")
                if status == "completed":
                    break
                await asyncio.sleep(60)

            if getattr(run, "conclusion", None) != "success":
                console.print("[red]本次提交构建失败，忽略[/red]")
                continue

            artifact, _ = find_latest_success_artifact(repo, workflow_file, artifact_name, commit_sha=repo_latest_sha)
            if not artifact:
                console.print("[red]构建成功但未找到 artifact[/red]")
                continue

            zip_path = os.path.join(DOWNLOADS_DIR, f"{artifact.name}-run{run.run_number}.zip")
            console.print(f"[green]下载 artifact: {zip_path}[/green]")
            download_with_progress(artifact.archive_download_url, GITHUB_API_KEY, zip_path)
            extract_and_index(zip_path)
            console.print(f"[bold green]索引完成，文件数: {len(INDEX.filename_to_path)}[/bold green]")

        except Exception as e:
            console.print(f"[red]轮询错误: {e}")
        finally:
            await asyncio.sleep(10)

async def initialize_server():
    """服务器启动时初始化，检查现有文件或下载最新构建"""
    console.print("[yellow]服务器初始化中...[/yellow]")
    
    # 首先检查是否已有提取的文件
    if os.path.exists(EXTRACT_DIR) and os.listdir(EXTRACT_DIR):
        console.print("[blue]发现现有文件，重建索引...[/blue]")
        INDEX.index_dir(EXTRACT_DIR)
        console.print(f"[green]索引完成，文件数: {len(INDEX.filename_to_path)}[/green]")
        return
    
    # 如果没有现有文件，尝试下载最新的成功构建
    try:
        console.print("[blue]没有现有文件，尝试下载最新构建...[/blue]")
        auth = Auth.Token(GITHUB_API_KEY)
        gh = Github(auth=auth)
        repo = gh.get_repo(GITHUB_REPO_URL)
        workflow_file = ".github/workflows/cli-build.yml"
        artifact_name = "Fatalder-cli-artifacts"
        
        # 查找最新的成功构建（不限制特定提交）
        artifact, run = find_latest_success_artifact(repo, workflow_file, artifact_name, any_commit=True)
        if artifact:
            zip_path = os.path.join(DOWNLOADS_DIR, f"{artifact.name}-run{run.run_number}.zip")
            console.print(f"[green]下载最新构建: {zip_path}[/green]")
            download_with_progress(artifact.archive_download_url, GITHUB_API_KEY, zip_path)
            extract_and_index(zip_path)
            console.print(f"[bold green]初始化完成，文件数: {len(INDEX.filename_to_path)}[/bold green]")
        else:
            console.print("[yellow]未找到可用的构建，等待新提交...[/yellow]")
    except Exception as e:
        console.print(f"[red]初始化失败: {e}[/red]")
        console.print("[yellow]将等待新提交触发下载...[/yellow]")

@app.on_event("startup")
async def on_startup():
    await initialize_server()
    asyncio.create_task(poll_commit_loop())

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8888, reload=False)
