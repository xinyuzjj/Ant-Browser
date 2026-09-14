import { projectConfig } from './projectBase.config'
import { PROJECT_GITHUB_URL } from './links'

export type ProfileIconKey =
  | 'book-open'
  | 'globe'
  | 'message-square'
  | 'github'
  | 'mail'
  | 'external-link'

export interface ProfileChannelConfig {
  name: string
  description: string
  detail: string
  href?: string
  icon?: ProfileIconKey
}

export interface AuthorProfileConfig {
  name: string
  initial: string
  title: string
  bio: string
  location: string
  joinDate: string
  email: string
  website: string
  github: string
  skills: string[]
  channels: ProfileChannelConfig[]
}

export interface ProjectProfileActionConfig {
  label: string
  href: string
  icon: ProfileIconKey
}

export interface ProjectProfileConfig {
  name: string
  introBadge: string
  introText: string
  techStack: string[]
  description: string
  actions: ProjectProfileActionConfig[]
}

export interface RemoteAuthorSourceConfig {
  authorURL: string
  timeoutMs: number
}

export interface ProfilePageLocalConfig {
  remoteAuthor: RemoteAuthorSourceConfig
  defaultAuthor: AuthorProfileConfig
  project: ProjectProfileConfig
}

export const profilePageConfig: ProfilePageLocalConfig = {
  remoteAuthor: {
    // 留空时直接使用本地默认资料；需要远程作者页时再替换为真实地址。
    // https://raw.githubusercontent.com/<user>/<repo>/main/author.json
    authorURL: '',
    timeoutMs: 1000,
  },
  defaultAuthor: {
    name: 'junjunni',
    initial: 'J',
    title: 'Ant Browser 维护者',
    bio: '维护 Ant Browser 分支，持续修复问题、改进使用体验。',
    location: '中国',
    joinDate: '',
    email: '1105002234@qq.com',
    website: '',
    github: 'https://github.com/xinyuzjj',
    skills: ['Go', 'React', 'TypeScript', 'Wails', 'Node.js', 'Docker'],
    channels: [
      {
        name: 'GitHub',
        description: '源码与提交记录',
        detail: 'xinyuzjj/Ant-Browser',
        href: 'https://github.com/xinyuzjj/Ant-Browser',
        icon: 'github',
      },
      {
        name: '发布版本',
        description: '安装包与便携包下载',
        detail: 'releases',
        href: 'https://github.com/xinyuzjj/Ant-Browser/releases',
        icon: 'globe',
      },
      {
        name: '问题反馈',
        description: '提交问题与建议',
        detail: 'issues',
        href: 'https://github.com/xinyuzjj/Ant-Browser/issues',
        icon: 'message-square',
      },
    ],
  },
  project: {
    name: projectConfig.name,
    introBadge: projectConfig.name,
    introText: '是一个面向多账号隔离、代理绑定和本地环境管理的桌面浏览器工具。',
    techStack: ['Wails', 'React', 'TypeScript'],
    description: '项目当前聚焦浏览器实例隔离、代理池配置、浏览器内核管理、标签检索和快捷启动等核心能力，适合跨境电商、社媒运营、本地测试以及需要统一管理浏览器环境的团队场景。',
    actions: [
      {
        label: '查看源码',
        href: PROJECT_GITHUB_URL,
        icon: 'github',
      },
      {
        label: '下载发布版',
        href: `${PROJECT_GITHUB_URL}/releases`,
        icon: 'globe',
      },
    ],
  },
}

export default profilePageConfig
