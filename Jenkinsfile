pipeline {
    agent any
    
    stages {
        stage('构建镜像') {
            steps {
                echo "✅ 开始构建镜像（所有分支都会执行）"
                // 核心：打印当前分支名（多分支流水线会自动填充env.BRANCH_NAME）
                echo "👉 当前运行分支：${env.BRANCH_NAME}"
                // 模拟构建镜像
                // sh "docker build -t my-app:${env.BRANCH_NAME} ."
            }
        }
        
        stage('推送镜像到仓库') {
            // 条件：仅main分支执行推送
            when {
                branch 'main' // 多分支下，这里会精准匹配main分支
            }
            steps {
                echo "🚀 ${env.BRANCH_NAME}分支，开始推送镜像！"
                // 模拟推送镜像（分支名作为标签）
                // sh "docker push my-app:${env.BRANCH_NAME}"
            }
        }

        // 可选：给dev分支加专属步骤
        stage('部署测试环境') {
            when {
                branch 'dev' // 仅dev分支执行
            }
            steps {
                echo "📝 ${env.BRANCH_NAME}分支，部署到测试环境！"
            }
        }
    }
}
