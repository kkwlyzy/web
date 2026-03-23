# Go 镜像转换器（x86 <-> amr/arm64）

这个工具用于在服务器上把一个镜像中的项目目录提取出来，放到另一个基础镜像里，并生成新镜像。

- 适用场景：`linux/amd64` 与 `linux/arm64` 互转
- 目录可自定义：源目录 `--src-dir`、目标目录 `--dst-dir`
- 支持别名：`x86`、`amd64`、`amr`、`arm64`

## 依赖

- Go 1.22+
- Docker（服务端可用）
### 注意
go.mod 文件第一行开头的 BOM 字符（十六进制为 EF BB BF）。在 Go 语言中，如果 go.mod 文件包含了这个标记，编译器有时会报错，
linux中需要删除mod中第一行
```sed -i '1s/^\xef\xbb\xbf//' go.mod```
## 构建

```bash
go build -o image-converter .
```

## 使用

```bash
./image-converter \
  --src-image registry.example.com/myapp:amd64 \
  --dst-image registry.example.com/runtime:arm64 \
  --out-image registry.example.com/myapp:arm64 \
  --src-dir /opt/project \
  --dst-dir /srv/project \
  --src-platform linux/amd64 \
  --dst-platform linux/arm64
```

## 参数说明

- `--src-image`：源镜像（提取项目）
- `--dst-image`：目标基础镜像（承载项目）
- `--out-image`：输出镜像标签
- `--src-dir`：源镜像中要提取的目录
- `--dst-dir`：写入目标镜像的目录
- `--src-platform`：源镜像平台（默认 `linux/amd64`）
- `--dst-platform`：目标镜像平台（默认 `linux/arm64`）
- `--keep-container`：失败时保留临时容器排查
- `-v`：输出详细日志（默认开启）

## 例子

1) `amd64 -> arm64`

```bash
./image-converter \
  --src-image app:v1-amd64 \
  --dst-image base:v1-arm64 \
  --out-image app:v1-arm64 \
  --src-dir /app \
  --dst-dir /app \
  --src-platform x86 \
  --dst-platform amr
```

2) `arm64 -> amd64`

```bash
./image-converter \
  --src-image app:v1-arm64 \
  --dst-image base:v1-amd64 \
  --out-image app:v1-amd64 \
  --src-dir /workspace/project \
  --dst-dir /opt/project \
  --src-platform arm64 \
  --dst-platform amd64
```

## 注意

- 该工具只做“目录迁移 + 新镜像封装”，不会自动编译二进制。
- 如果项目里有架构相关可执行文件，你需要在目标架构下重新构建。
- 生成完成后可用 `docker push <out-image>` 推送到仓库。
