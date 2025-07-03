package utils

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/ssh"
)

// SSHClient 封装 SSH 连接
type SSHClient struct {
	client  *ssh.Client
	host    string
	user    string
}

// NewSSHClient 创建一个新的 SSH 客户端
func NewSSHClient(host, user, password, keyPath string) (*SSHClient, error) {
	var auth []ssh.AuthMethod

	// 如果提供了密钥路径，优先使用密钥认证
	if keyPath != "" {
		key, err := ioutil.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("读取SSH密钥失败: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("解析SSH密钥失败: %w", err)
		}

		auth = append(auth, ssh.PublicKeys(signer))
	} else if password != "" {
		// 否则使用密码认证
		auth = append(auth, ssh.Password(password))
	} else {
		return nil, fmt.Errorf("必须提供密码或SSH密钥")
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", host), config)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}

	return &SSHClient{
		client: client,
		host:   host,
		user:   user,
	}, nil
}

// Close 关闭 SSH 连接
func (s *SSHClient) Close() error {
	return s.client.Close()
}

// RunCommand 执行命令并返回输出
func (s *SSHClient) RunCommand(command string) (string, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	if err != nil {
		return "", fmt.Errorf("执行命令失败: %v, 错误: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// UploadFile 上传文件到远程服务器
func (s *SSHClient) UploadFile(localPath, remotePath string) error {
	// 打开本地文件
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer localFile.Close()

	// 创建新会话
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	// 设置远程命令，创建远程文件
	remoteCmd := fmt.Sprintf("cat > %s", remotePath)
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取标准输入管道失败: %w", err)
	}

	// 设置输出管道
	var stderr bytes.Buffer
	session.Stderr = &stderr

	// 启动远程命令
	if err := session.Start(remoteCmd); err != nil {
		return fmt.Errorf("启动远程命令失败: %w", err)
	}

	// 复制文件内容到远程
	_, err = io.Copy(stdin, localFile)
	if err != nil {
		return fmt.Errorf("上传文件内容失败: %w", err)
	}

	// 关闭输入流
	stdin.Close()

	// 等待命令完成
	if err := session.Wait(); err != nil {
		return fmt.Errorf("上传文件失败: %v, 错误: %s", err, stderr.String())
	}

	fmt.Printf("文件 %s 上传至 %s:%s 成功\n", localPath, s.host, remotePath)
	return nil
}

// UploadString 将字符串上传为远程文件
func (s *SSHClient) UploadString(content, remotePath string) error {
	// 创建新会话
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	// 设置远程命令，创建远程文件
	remoteCmd := fmt.Sprintf("cat > %s", remotePath)
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取标准输入管道失败: %w", err)
	}

	// 设置输出管道
	var stderr bytes.Buffer
	session.Stderr = &stderr

	// 启动远程命令
	if err := session.Start(remoteCmd); err != nil {
		return fmt.Errorf("启动远程命令失败: %w", err)
	}

	// 写入内容到远程文件
	_, err = stdin.Write([]byte(content))
	if err != nil {
		return fmt.Errorf("写入文件内容失败: %w", err)
	}

	// 关闭输入流
	stdin.Close()

	// 等待命令完成
	if err := session.Wait(); err != nil {
		return fmt.Errorf("上传文件失败: %v, 错误: %s", err, stderr.String())
	}

	fmt.Printf("内容已上传至 %s:%s\n", s.host, remotePath)
	return nil
} 