import { words } from "./format";

// Guide is the in-app manual, field by field: every form in the app, what to put in each field
// and what it does. One card per page, one table per form.
type Field = [string, string, string]; // 字段, 怎么填, 作用
type Form = { name: string; note?: string; fields: Field[] };
type Page = { title: string; intro?: string; forms: Form[] };

const pages: Page[] = [
  {
    title: "VPN",
    intro: "每个 VPN 是一个节点加几条规则。连上以后在「策略」和「规则」页都能看到，退出应用时自动断开。",
    forms: [
      {
        name: "编辑 VPN：通用字段",
        fields: [
          ["名称", "任意，不能和其他 VPN 重名", "节点名，规则和状态栏都用它"],
          ["类型", "EasyConnect / WireGuard / OpenVPN / Tailscale", "决定下面出现哪些字段；连上后不能改"],
          ["连接 / 保存并重连", "填完点一下", "写入配置并连接；已连接的改完重新连"],
          ["断开", "—", "撤掉配置，设置留着，下次点「连接」即可"],
          ["删除", "—", "连同保存的设置一起去掉"],
        ],
      },
      {
        name: "EasyConnect（深信服 SSL VPN）",
        fields: [
          ["网关地址", "如 vpn.example.edu.cn，不带 https://", "登录和隧道都连它"],
          ["端口", "默认 443", "网关端口"],
          ["用户名 / 密码", "校园网或公司网账号", "登录"],
          ["TOTP 密钥", "账号开了二次验证时填 base32 密钥，没有就留空", "自动生成验证码；短信和图片验证码不支持"],
          ["测试登录", "填好账号后点", "真实登录一次并立刻登出，报告隧道地址和网关路由数量，不保存"],
        ],
      },
      {
        name: "WireGuard",
        fields: [
          ["服务器地址 / 端口", "对端的公网地址和 UDP 端口", "隧道对端"],
          ["本机私钥", "你的 WireGuard 私钥（base64）", "本机身份"],
          ["对端公钥", "服务器的公钥", "验证对端"],
          ["预共享密钥", "可选", "多一层对称加密"],
          ["本机地址", "如 10.0.0.2", "隧道里本机的 IP"],
          ["MTU", "可选，默认 1408", "隧道包大小"],
          ["对端网段", "allowed-ips，逗号分隔，如 10.0.0.0/8", "这些网段走隧道；不填则按下面的附加路由"],
        ],
      },
      {
        name: "OpenVPN",
        fields: [
          ["配置文件", "把 .ovpn 文件内容整个粘进来", "解析出服务器、证书和密钥"],
          ["用户名 / 密码", "服务端要求时填", "登录"],
        ],
      },
      {
        name: "Tailscale",
        fields: [
          ["Auth key", "从 Tailscale 控制台生成，可选", "免交互登录；不填时在「日志」里找登录链接"],
          ["本机名字", "可选", "在 tailnet 里显示的主机名"],
          ["控制服务器", "用 Headscale 时填它的地址", "默认是官方控制面"],
          ["出口节点", "可选，另一台机器的名字", "把流量从那台机器出去"],
          ["接受子网路由", "勾上", "使用别的节点宣告的子网"],
        ],
      },
      {
        name: "直连网络",
        note: "在这些网络里不走 VPN，比如人已经在校内。有条件时会生成一个按网络自动选择的策略组。",
        fields: [
          ["Wi-Fi 名称", "SSID；点「当前网络」自动填", "连着这个 Wi-Fi 时直连；需要管理员权限的内核才能读到 SSID"],
          ["Wi-Fi BSSID", "路由器的 MAC，如 aa:bb:cc:dd:ee:ff", "同名 Wi-Fi 有多个时精确到一台"],
          ["网关地址", "默认网关 IP，如 192.168.1.1", "不需要权限的判断方式"],
          ["本机网段", "如 192.168.1.0/24", "本机 IP 落在网段内时直连"],
          ["任何 Wi-Fi / 任何有线", "不填值", "按连接方式"],
        ],
      },
      {
        name: "附加路由",
        note: "EasyConnect 自动走网关下发的网段和域名，这里是额外的；其他类型全靠这里。",
        fields: [
          ["网段", "10.0.0.0/8，或单个 IP", "目标 IP 在网段内走 VPN，不解析域名"],
          ["域名后缀", "example.edu.cn", "该域名及其子域名"],
          ["域名", "git.example.edu.cn", "只这一个域名"],
          ["域名关键字", "example", "域名里含这个词"],
          ["GEOSITE / GEOIP", "如 cn", "按内置地理库"],
        ],
      },
      {
        name: "测试",
        fields: [["目标地址", "主机:端口，如 10.1.2.3:22", "经本机代理端口真连一次，报告耗时、命中的规则和出口节点"]],
      },
    ],
  },
  {
    title: "策略",
    forms: [
      {
        name: "添加节点：通用字段",
        fields: [
          ["名称", "必填，不重名", "规则里用它"],
          ["协议", "下拉选", "决定下面的字段"],
          ["服务器地址 / 端口", "域名或 IP，加端口", "连哪里"],
          ["允许 UDP", "勾上", "游戏、语音等 UDP 流量也走它"],
          ["TCP Fast Open", "一般不勾", "服务端支持时少一次往返"],
          ["IP 版本", "默认自动", "强制 IPv4 或 IPv6"],
          ["网络接口", "可选，如 en0", "从指定网卡连出去"],
          ["跳板代理", "选另一个节点或 VPN", "先经它再连本节点，例如只有校内能访问的代理经 VPN"],
        ],
      },
      {
        name: "各协议字段",
        fields: [
          ["HTTP / SOCKS5", "用户名、密码、TLS、SNI、跳过证书验证", "普通代理"],
          ["Shadowsocks", "加密方式、密码", "和服务端一致"],
          ["VMess", "UUID、alterId、加密、TLS、SNI、传输方式（tcp/ws/grpc/h2/http）及其路径、Host、服务名", "照服务端配置填"],
          ["VLESS", "UUID、Flow、TLS、SNI、指纹、Reality 公钥和 short-id、传输方式", "Reality 时填公钥和 short-id"],
          ["Trojan", "密码、SNI、跳过证书验证、传输方式", "—"],
          ["Hysteria2", "密码、SNI、上行/下行带宽（如 50 Mbps）、混淆及密码", "带宽影响拥塞控制，按实际填"],
          ["TUIC", "UUID、密码、SNI、拥塞控制", "—"],
          ["AnyTLS", "密码、SNI", "—"],
          ["SSH", "用户名、密码或私钥内容", "把 SSH 服务器当代理"],
          ["WireGuard", "私钥、对端公钥、IPv4/IPv6 地址、MTU", "作为普通节点；要路由规则时用 VPN 页"],
        ],
      },
      {
        name: "添加策略组",
        fields: [
          ["名称", "必填", "规则里用它"],
          ["类型", "手动选择 / 自动测速 / 故障转移 / 负载均衡 / 按网络自动选择", "怎么从成员里挑"],
          ["测试 URL / 间隔", "自动测速、故障转移、负载均衡用，默认即可", "探测用的地址和秒数"],
          ["成员", "勾选节点或其他组，DIRECT、REJECT 也可以", "候选"],
          ["默认 / 条件→策略", "按网络自动选择时：默认成员，以及 SSID:名称、ROUTER:网关、网段 等条件对应的成员", "换网络自动切"],
        ],
      },
    ],
  },
  {
    title: "规则",
    intro: "从上到下匹配，第一条命中的决定出口。自己加的排最前，可上下移动；订阅里的可以隐藏。",
    forms: [
      {
        name: "增加规则",
        fields: [
          ["规则类型", "DOMAIN-SUFFIX / DOMAIN / DOMAIN-KEYWORD / DOMAIN-REGEX / DOMAIN-WILDCARD / GEOSITE / IP-CIDR / IP-CIDR6 / SRC-IP-CIDR / GEOIP / IP-ASN / DST-PORT / SRC-PORT / PROCESS-NAME / PROCESS-PATH / PROCESS-ANCESTOR / PROCESS-ANCESTOR-PATH / IN-NAME / IN-TYPE / NETWORK / RULE-SET", "按域名、IP、端口、进程、入站或网络类型匹配"],
          ["值", "如 google.com、10.0.0.0/8、ghostty", "匹配什么"],
          ["策略", "节点、策略组、DIRECT 或 REJECT", "命中后走哪里"],
          ["no-resolve", "IP 类规则默认勾上", "不为了匹配去解析域名，避免拖慢和泄漏"],
          ["EASYCONNECT,<节点>,<策略>", "VPN 页自动写入", "匹配该 VPN 网关下发的网段和域名"],
        ],
      },
      {
        name: "临时规则（连接页）",
        fields: [["按主机 / 来源加临时规则", "在连接详情里点", "立刻生效、不写入配置，重启内核后消失"]],
      },
    ],
  },
  {
    title: "反向",
    intro: "把本机服务经 FRP 或 SSH 暴露到远端服务器，外面的设备连那台服务器就能用这台电脑的代理。",
    forms: [
      {
        name: "增加隧道：FRP",
        fields: [
          ["名称", "必填，字母数字", "隧道名，也用于入站规则"],
          ["frps 地址 / 端口", "你的 frps 服务器，默认端口 7000", "连哪台 frps"],
          ["Token", "frps 的 auth.token，可选", "认证"],
          ["传输协议", "tcp / kcp / quic / websocket", "和 frps 一致"],
        ],
      },
      {
        name: "增加隧道：SSH",
        fields: [
          ["SSH 服务器 / 端口", "服务器地址，默认 22", "用 ssh -R 反向转发"],
          ["用户名", "服务器账号", "—"],
          ["认证方式", "密码，或粘贴私钥内容", "登录"],
          ["服务器公钥", "点「获取」自动填", "校验服务器身份，防中间人"],
        ],
      },
      {
        name: "转发与分流",
        fields: [
          ["转发（远端监听 → 本地目标）", "远端端口，本地 地址:端口；默认指向本机代理端口", "外面连远端端口，到本机这个目标"],
          ["跳板代理", "服务器在 VPN 或代理后面时选对应节点", "隧道自己的连接经它出去"],
          ["入站流量按规则分流", "勾上", "进来的连接按规则走，而不是直接到本地目标"],
        ],
      },
      {
        name: "部署 frps",
        fields: [
          ["服务器 / 端口 / 用户名", "你的 Linux 服务器 SSH 信息，需要 root 或免密 sudo", "一键安装最新 frps 并设开机启动"],
          ["认证方式", "密码或私钥", "—"],
          ["frps 端口", "默认 7000", "装好后隧道连这个端口"],
        ],
      },
    ],
  },
  {
    title: "设备",
    forms: [
      {
        name: "局域网共享",
        fields: [
          ["允许局域网设备连接", "打开", "同一网络的设备把代理填成这台电脑的 IP 和代理端口即可"],
          ["设备的策略", "在设备上选", "把这台设备的流量固定到某个节点或策略组"],
        ],
      },
    ],
  },
  {
    title: "增强",
    forms: [
      {
        name: "接管方式",
        fields: [
          ["增强模式", "先在「通用」授予权限，再打开", "TUN 接管全部流量，包括不认系统代理的程序"],
          ["部分增强", "打开后勾选应用", "只接管勾选的应用及其子进程，其余照常"],
          ["应用列表", "正在运行的应用、有连接的进程、常用命令行工具（ssh、git、curl、pip、npm…）", "勾上即接管"],
          [words.pickApp, "从磁盘选一个应用", "没在列表里的也能加"],
        ],
      },
    ],
  },
  {
    title: "配置",
    forms: [
      {
        name: "配置来源",
        fields: [
          ["添加订阅", "名称可选，订阅链接必填", "定期更新的远程配置"],
          ["导入文件 / 本地配置", "选 YAML 文件", "手写的配置"],
          ["当前配置", "单击选中，双击使用", "内核用哪一份"],
        ],
      },
      {
        name: "覆写",
        note: "叠加在当前配置之上，订阅更新不会冲掉；界面添加的节点、组、规则、隧道、VPN 都在这里。",
        fields: [
          ["字段 / 值", "如 dns.enable 和 true、mixed-port 和 7890、[a, b]", "快速改一个键"],
          ["编辑器", "直接改 YAML", "prepend-rules、prepend-proxies、prepend-proxy-groups、remove-rules、patch-proxies、group-members、reverse-tunnels、capture 等"],
        ],
      },
    ],
  },
  {
    title: "DNS",
    forms: [
      {
        name: "设置",
        fields: [
          ["启用 DNS", "增强模式下必须", "内核自带的解析"],
          ["模式", "FAKE-IP / REDIR-HOST", "fake-ip 用假地址保住域名、更快；redir-host 真实解析"],
          ["解析 IPv6", "按网络环境", "是否返回 AAAA"],
          ["DoH 优先使用 HTTP/3", "可选", "上游是 DoH 时先试 H3"],
          ["DNS 查询遵循规则", "建议开", "解析也按规则走代理，减少泄漏"],
          ["上游服务器", "如 https://doh.pub/dns-query 或 223.5.5.5，一行一个", "问谁"],
          ["Fake-IP 地址段", "默认 198.18.0.1/16", "假地址从这里分配"],
          ["跟随系统分域名 DNS", "开", "VPN 等为特定域名设置的 DNS 继续生效"],
          ["使用系统 HOSTS", "开", "/etc/hosts 生效"],
          ["DNS 泄漏检测", "点一下", "看解析是经代理还是直连"],
        ],
      },
    ],
  },
  {
    title: "通用与关于",
    forms: [
      {
        name: "通用",
        fields: [
          ["代理端口", "默认 7899", "HTTP 与 SOCKS5 共用的本机端口"],
          ["日志级别", "info 够用，排错时 debug", "日志页显示多少"],
          ["重启内核", "—", "重新启动内核并载入当前配置"],
          ["root 权限", "授予一次", "装管理员权限的内核，增强模式需要；移除则回到普通内核"],
          ["开机时启动", "—", "登录后自动打开"],
          [`${words.tray}显示网速`, "—", "图标旁显示上下行"],
          ["检查更新", "—", "从 GitHub Releases 下载、校验、安装"],
        ],
      },
      {
        name: "关于",
        fields: [["版本 / 内核 / 内核运行方式 / 数据目录", "只读", "排错时看这里；数据目录里有 config.yaml、core.log、profiles/"]],
      },
    ],
  },
  {
    title: "活动",
    forms: [
      {
        name: "工具",
        fields: [
          ["网络诊断", "点一下，可中途终止", "逐项检查配置、本机网络、路由、DNS、直连和代理"],
          ["网络修复", "有问题的项点修复，或「全部修复」", "清指向已退出程序的系统代理、结束残留内核、移除残留 TUN；刷新 DNS 缓存；Windows 还有 Winsock 和 TCP/IP 重置"],
          ["INTERNET 延迟 ↻", "点一下", "重测路由、DNS 和代理延迟"],
        ],
      },
    ],
  },
];

export default function Guide() {
  return (
    <>
      <div className="hint">每个页面的每个字段：填什么、有什么用。</div>
      {pages.map((p) => (
        <div key={p.title} className="card" style={{ marginTop: 16 }}>
          <div className="label">{p.title}</div>
          {p.intro && <p className="small muted" style={{ margin: "6px 0 0", lineHeight: 1.6 }}>{p.intro}</p>}
          {p.forms.map((f) => (
            <div key={f.name}>
              <div className="section-title" style={{ fontSize: 13 }}>{f.name}</div>
              {f.note && <p className="small muted" style={{ margin: "0 0 4px", lineHeight: 1.6 }}>{f.note}</p>}
              <div className="table-wrap tight" style={{ marginTop: 4 }}>
                <table className="table guide">
                  <thead><tr><th>字段</th><th>怎么填</th><th>作用</th></tr></thead>
                  <tbody>{f.fields.map(([k, how, why]) => <tr key={k}><td>{k}</td><td>{how}</td><td>{why}</td></tr>)}</tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      ))}
    </>
  );
}
