# Python API 参考

本页面提供 AgentFense Python SDK 的 API 参考概览。完整的 API 文档请参阅 [英文版 API 参考](../../reference/python.md)。

## 高层 API

### Sandbox 类

主要的沙盒操作类，提供简化的 API。

```python
from agentfense import Sandbox
```

**类方法：**

| 方法 | 描述 |
|------|------|
| `from_local(path, ...)` | 从本地目录创建沙盒 |
| `from_codebase(codebase_id, ...)` | 从已有代码库创建沙盒 |
| `connect(sandbox_id)` | 连接到现有沙盒 |

**实例方法：**

| 方法 | 描述 |
|------|------|
| `run(command, ...)` | 执行命令（简化版） |
| `exec(command, ...)` | 执行命令（完整参数） |
| `exec_stream(command, ...)` | 流式执行命令 |
| `session(shell, env)` | 创建持久会话 |
| `read_file(path)` | 读取文件 |
| `write_file(path, content)` | 写入文件 |
| `list_files(path, recursive)` | 列出文件 |
| `start()` | 启动沙盒 |
| `stop()` | 停止沙盒 |
| `destroy(delete_codebase)` | 销毁沙盒 |

### AsyncSandbox 类

异步版本的 Sandbox，API 与同步版本相同，但所有方法都是异步的。

```python
from agentfense import AsyncSandbox

async with await AsyncSandbox.from_local("./project") as sandbox:
    result = await sandbox.run("python main.py")
```

## 低层 API

### SandboxClient 类

提供对服务端的完整控制。

```python
from agentfense import SandboxClient

client = SandboxClient(endpoint="localhost:9000")
```

**沙盒管理：**

| 方法 | 描述 |
|------|------|
| `create_sandbox(...)` | 创建沙盒 |
| `get_sandbox(sandbox_id)` | 获取沙盒信息 |
| `list_sandboxes(codebase_id)` | 列出沙盒 |
| `start_sandbox(sandbox_id)` | 启动沙盒 |
| `stop_sandbox(sandbox_id)` | 停止沙盒 |
| `destroy_sandbox(sandbox_id)` | 销毁沙盒 |

**命令执行：**

| 方法 | 描述 |
|------|------|
| `exec(sandbox_id, command, ...)` | 执行命令 |
| `exec_stream(sandbox_id, command, ...)` | 流式执行 |

**会话管理：**

| 方法 | 描述 |
|------|------|
| `create_session(sandbox_id, ...)` | 创建会话 |
| `session_exec(session_id, command)` | 在会话中执行 |
| `destroy_session(session_id)` | 销毁会话 |

**代码库管理：**

| 方法 | 描述 |
|------|------|
| `create_codebase(name, owner_id)` | 创建代码库 |
| `get_codebase(codebase_id)` | 获取代码库 |
| `delete_codebase(codebase_id)` | 删除代码库 |
| `upload_file(codebase_id, path, content)` | 上传文件 |
| `download_file(codebase_id, path)` | 下载文件 |
| `list_files(codebase_id, path, recursive)` | 列出文件 |

## 类型定义

### Permission 枚举

```python
from agentfense import Permission

Permission.NONE   # 不可见
Permission.VIEW   # 仅列表
Permission.READ   # 可读
Permission.WRITE  # 可写
```

### PatternType 枚举

```python
from agentfense import PatternType

PatternType.GLOB       # 通配符模式，如 **/*.py
PatternType.DIRECTORY  # 目录前缀，如 /docs/
PatternType.FILE       # 精确文件，如 /config.yaml
```

### RuntimeType 枚举

```python
from agentfense import RuntimeType

RuntimeType.BWRAP   # bubblewrap（轻量）
RuntimeType.DOCKER  # Docker 容器（完整隔离）
```

### PermissionRule 数据类

```python
from agentfense import PermissionRule, Permission, PatternType

rule = PermissionRule(
    pattern="**/*.py",
    permission=Permission.READ,
    type=PatternType.GLOB,
    priority=0  # 可选，自动计算
)
```

### ResourceLimits 数据类

```python
from agentfense import ResourceLimits

limits = ResourceLimits(
    memory_bytes=512 * 1024 * 1024,  # 512 MB
    cpu_quota=50000,                  # 50% CPU
    pids_limit=100                    # 最多 100 个进程
)
```

### ExecResult 数据类

```python
@dataclass
class ExecResult:
    stdout: str       # 标准输出
    stderr: str       # 标准错误
    exit_code: int    # 退出码
    duration: float   # 执行时间（秒）
```

## 异常类

### 异常层次结构

```
SandboxError (基类)
├── ConnectionError          # 连接失败
├── InvalidConfigurationError # 配置无效
├── SandboxNotFoundError     # 沙盒不存在
├── SandboxNotRunningError   # 沙盒未运行
├── CommandTimeoutError      # 命令超时
├── CommandExecutionError    # 命令执行失败
├── PermissionDeniedError    # 权限被拒绝
├── SessionError             # 会话错误
│   ├── SessionNotFoundError # 会话不存在
│   └── SessionClosedError   # 会话已关闭
├── CodebaseError            # 代码库错误
│   ├── CodebaseNotFoundError # 代码库不存在
│   ├── FileNotFoundError    # 文件不存在
│   └── UploadError          # 上传失败
└── ResourceLimitExceededError # 资源超限
```

### 常用异常处理

```python
from agentfense import (
    Sandbox,
    SandboxError,
    CommandTimeoutError,
    CommandExecutionError,
)

try:
    with Sandbox.from_local("./project") as sandbox:
        result = sandbox.run("python main.py", timeout=30, raise_on_error=True)
except CommandTimeoutError:
    print("命令超时")
except CommandExecutionError as e:
    print(f"命令失败 (exit {e.exit_code}): {e.stderr}")
except SandboxError as e:
    print(f"沙盒错误: {e}")
```

## 预设函数

```python
from agentfense import list_presets, get_preset, extend_preset, register_preset

# 列出所有预设
list_presets()  # ['agent-safe', 'read-only', 'full-access', 'development', 'view-only']

# 获取预设规则
rules = get_preset("agent-safe")

# 扩展预设
rules = extend_preset("agent-safe", additions=[
    {"pattern": "/custom/**", "permission": "write"}
])

# 注册自定义预设
register_preset("my-preset", [
    {"pattern": "**/*", "permission": "read"},
    {"pattern": "/output/**", "permission": "write"},
])
```

## 工具函数

```python
from agentfense.utils import (
    walk_directory,      # 遍历目录
    parse_ignore_file,   # 解析 .gitignore
    human_readable_size, # 人类可读文件大小
    generate_codebase_name,  # 生成代码库名称
    count_files,         # 统计文件数量
)
```

## 更多信息

- [Python SDK 使用指南](../sdk/python/overview.md)
- [高层 API 详解](../sdk/python/high-level.md)
- [异步 API](../sdk/python/async.md)
- [错误处理](../sdk/python/errors.md)
