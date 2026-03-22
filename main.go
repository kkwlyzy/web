package main

import (
	"archive/tar"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
)

type options struct {
	sourceImage   string
	targetImage   string
	outputImage   string
	sourceDir     string
	targetDir     string
	sourcePlat    string
	targetPlat    string
	keepContainer bool
	verbose       bool
}

// 检查本地是否存在镜像
func imageExists(image string) bool {
	cmd := exec.Command("docker", "inspect", "--type=image", image)
	return cmd.Run() == nil
}

func main() {
	opts, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		os.Exit(2)
	}

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "转换失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("转换完成: %s -> %s\n", opts.sourceImage, opts.outputImage)
}

func parseFlags() (*options, error) {
	opts := &options{}

	flag.StringVar(&opts.sourceImage, "src-image", "", "源镜像，例如: registry/app:amd64")
	flag.StringVar(&opts.targetImage, "dst-image", "", "目标基础镜像，例如: registry/base:arm64")
	flag.StringVar(&opts.outputImage, "out-image", "", "输出镜像标签")
	flag.StringVar(&opts.sourceDir, "src-dir", "", "源镜像中的项目目录，例如: /opt/project")
	flag.StringVar(&opts.targetDir, "dst-dir", "", "写入目标镜像的目录，例如: /srv/app")
	flag.StringVar(&opts.sourcePlat, "src-platform", "linux/amd64", "源镜像平台，支持别名: x86/amd64/arm/arm64")
	flag.StringVar(&opts.targetPlat, "dst-platform", "linux/arm64", "目标镜像平台，支持别名: x86/amd64/arm/arm64")
	flag.BoolVar(&opts.keepContainer, "keep-container", false, "失败时保留临时容器用于排查")
	flag.BoolVar(&opts.verbose, "v", true, "打印执行日志")

	flag.Parse()

	if opts.sourceImage == "" || opts.targetImage == "" || opts.outputImage == "" {
		return nil, errors.New("--src-image --dst-image --out-image 都是必填")
	}
	if opts.sourceDir == "" || opts.targetDir == "" {
		return nil, errors.New("--src-dir --dst-dir 都是必填")
	}

	var err error
	opts.sourceDir, err = normalizeContainerDir(opts.sourceDir)
	if err != nil {
		return nil, fmt.Errorf("--src-dir 无效: %w", err)
	}
	opts.targetDir, err = normalizeContainerDir(opts.targetDir)
	if err != nil {
		return nil, fmt.Errorf("--dst-dir 无效: %w", err)
	}

	opts.sourcePlat, err = normalizePlatform(opts.sourcePlat)
	if err != nil {
		return nil, fmt.Errorf("--src-platform 无效: %w", err)
	}
	opts.targetPlat, err = normalizePlatform(opts.targetPlat)
	if err != nil {
		return nil, fmt.Errorf("--dst-platform 无效: %w", err)
	}

	return opts, nil
}

// 检查本地是否存在镜像
func imageExists(image string) bool {
	// 使用 docker inspect 检查镜像元数据，如果返回 0 说明本地已存在
	cmd := exec.Command("docker", "inspect", "--type=image", image)
	return cmd.Run() == nil
}

func run(opts *options) error {
	if err := checkDocker(); err != nil {
		return err
	}

	// 1. 处理源镜像：本地优先策略
	if imageExists(opts.sourceImage) {
		if opts.verbose {
			fmt.Printf("检测到本地源镜像: %s，跳过拉取阶段\n", opts.sourceImage)
		}
	} else {
		if opts.verbose {
			fmt.Printf("本地未找到源镜像，尝试从远程拉取: %s (%s)\n", opts.sourceImage, opts.sourcePlat)
		}
		if err := dockerPull(opts.sourceImage, opts.sourcePlat, opts.verbose); err != nil {
			return fmt.Errorf("拉取源镜像失败: %w", err)
		}
	}

	// 2. 处理目标基础镜像：本地优先策略
	if imageExists(opts.targetImage) {
		if opts.verbose {
			fmt.Printf("检测到本地目标镜像: %s，跳过拉取阶段\n", opts.targetImage)
		}
	} else {
		if opts.verbose {
			fmt.Printf("本地未找到目标镜像，尝试从远程拉取: %s (%s)\n", opts.targetImage, opts.targetPlat)
		}
		if err := dockerPull(opts.targetImage, opts.targetPlat, opts.verbose); err != nil {
			return fmt.Errorf("拉取目标镜像失败: %w", err)
		}
	}

	// 3. 创建源容器（用于提取文件）
	srcID, err := dockerCreate(opts.sourceImage, opts.sourcePlat)
	if err != nil {
		return fmt.Errorf("创建源容器失败: %w", err)
	}
	if !opts.keepContainer {
		defer dockerRm(srcID)
	}

	// 4. 创建目标容器（用于接收文件）
	dstID, err := dockerCreate(opts.targetImage, opts.targetPlat)
	if err != nil {
		return fmt.Errorf("创建目标容器失败: %w", err)
	}
	if !opts.keepContainer {
		defer dockerRm(dstID)
	}

	// 5. 执行目录迁移
	if opts.verbose {
		fmt.Printf("正在迁移目录: [%s]%s -> [%s]%s\n", opts.sourceImage, opts.sourceDir, opts.targetImage, opts.targetDir)
	}
	if err := streamProjectDir(srcID, sourceCopyPath(opts.sourceDir), dstID, opts.targetDir); err != nil {
		return fmt.Errorf("目录迁移失败: %w", err)
	}

	// 6. 将修改后的目标容器提交为新镜像
	if opts.verbose {
		fmt.Printf("正在生成新镜像: %s\n", opts.outputImage)
	}
	if err := dockerCommit(dstID, opts.outputImage); err != nil {
		return fmt.Errorf("提交镜像失败: %w", err)
	}

	return nil
}

func sourceCopyPath(dir string) string {
	if dir == "/" {
		return "/."
	}
	return strings.TrimSuffix(dir, "/") + "/."
}

func checkDocker() error {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("未检测到可用 Docker 服务: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dockerPull(image, platform string, verbose bool) error {
	args := []string{"pull", "--platform", platform, image}
	cmd := exec.Command("docker", args...)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker %s: %v", strings.Join(args, " "), err)
		}
		return nil
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dockerCreate(image, platform string) (string, error) {
	args := []string{"create", "--platform", platform, image}
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", errors.New("docker create 返回空容器 ID")
	}
	return id, nil
}

func streamProjectDir(srcContainerID, srcPath, dstContainerID, dstDir string) error {
	srcArgs := []string{"cp", srcContainerID + ":" + srcPath, "-"}
	dstArgs := []string{"cp", "-", dstContainerID + ":/"}

	srcCmd := exec.Command("docker", srcArgs...)
	dstCmd := exec.Command("docker", dstArgs...)

	srcStdout, err := srcCmd.StdoutPipe()
	if err != nil {
		return err
	}
	dstStdin, err := dstCmd.StdinPipe()
	if err != nil {
		return err
	}

	var srcErrBuf, dstErrBuf strings.Builder
	srcCmd.Stderr = &srcErrBuf
	dstCmd.Stderr = &dstErrBuf

	if err := dstCmd.Start(); err != nil {
		return fmt.Errorf("docker %s: %v", strings.Join(dstArgs, " "), err)
	}
	if err := srcCmd.Start(); err != nil {
		_ = dstStdin.Close()
		_ = dstCmd.Wait()
		return fmt.Errorf("docker %s: %v", strings.Join(srcArgs, " "), err)
	}

	rewriteErr := rewriteTarRoot(srcStdout, dstStdin, dstDir)
	closeErr := dstStdin.Close()
	if rewriteErr != nil {
		if srcCmd.Process != nil {
			_ = srcCmd.Process.Kill()
		}
		if dstCmd.Process != nil {
			_ = dstCmd.Process.Kill()
		}
		_ = srcCmd.Wait()
		_ = dstCmd.Wait()
		return rewriteErr
	}

	srcWaitErr := srcCmd.Wait()
	dstWaitErr := dstCmd.Wait()
	if closeErr != nil {
		return closeErr
	}
	if srcWaitErr != nil {
		msg := strings.TrimSpace(srcErrBuf.String())
		if msg != "" {
			return fmt.Errorf("docker %s: %v (%s)", strings.Join(srcArgs, " "), srcWaitErr, msg)
		}
		return fmt.Errorf("docker %s: %v", strings.Join(srcArgs, " "), srcWaitErr)
	}
	if dstWaitErr != nil {
		msg := strings.TrimSpace(dstErrBuf.String())
		if msg != "" {
			return fmt.Errorf("docker %s: %v (%s)", strings.Join(dstArgs, " "), dstWaitErr, msg)
		}
		return fmt.Errorf("docker %s: %v", strings.Join(dstArgs, " "), dstWaitErr)
	}

	return nil
}

func dockerCommit(containerID, outputImage string) error {
	args := []string{"commit", containerID, outputImage}
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dockerRm(containerID string) {
	_ = exec.Command("docker", "rm", "-f", containerID).Run()
}

func rewriteTarRoot(src io.Reader, dst io.Writer, dstDir string) error {
	tw := tar.NewWriter(dst)
	defer tw.Close()

	tr := tar.NewReader(src)
	entries := 0

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if hdr == nil {
			continue
		}

		name := cleanTarName(hdr.Name)
		if name == "" {
			continue
		}

		newHdr := *hdr
		newHdr.Name = toTarPath(path.Join(dstDir, name))
		if newHdr.Typeflag == tar.TypeLink {
			newHdr.Linkname = cleanTarName(newHdr.Linkname)
		}

		if err := tw.WriteHeader(&newHdr); err != nil {
			return err
		}

		if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			if _, err := io.Copy(tw, tr); err != nil {
				return err
			}
		}

		entries++
	}

	if entries == 0 {
		return errors.New("源目录为空，未找到可写入文件")
	}

	return nil
}

func normalizeContainerDir(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("目录不能为空")
	}
	if strings.Contains(raw, "\\") {
		return "", errors.New("容器内目录请使用 '/'，不要使用 '\\'")
	}
	return path.Clean("/" + strings.TrimPrefix(raw, "/")), nil
}

func normalizePlatform(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", errors.New("平台不能为空")
	}

	parts := strings.Split(raw, "/")
	var osName, arch string
	switch len(parts) {
	case 1:
		osName = "linux"
		arch = parts[0]
	case 2:
		osName = parts[0]
		arch = parts[1]
	default:
		return "", fmt.Errorf("不支持的平台格式: %s", raw)
	}

	if osName == "" {
		osName = "linux"
	}
	if osName != "linux" {
		return "", fmt.Errorf("当前仅支持 linux 平台，收到: %s", osName)
	}

	switch arch {
	case "amd64", "x86_64", "x64", "x86":
		arch = "amd64"
	case "arm64", "aarch64", "armv8", "arm", "amr", "amr64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("不支持的架构: %s，仅支持 x86/amd64 和 amr(arm64)", arch)
	}

	return osName + "/" + arch, nil
}

func cleanTarName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimPrefix(name, "/")
	name = path.Clean(name)
	if name == "." {
		return ""
	}
	return strings.TrimPrefix(name, "/")
}

func toTarPath(p string) string {
	p = path.Clean("/" + strings.TrimSpace(p))
	return strings.TrimPrefix(p, "/")
}
