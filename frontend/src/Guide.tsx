import { words } from "./format";

// Guide is the in-app manual: what each page does, how the pieces connect, and the config the
// core actually reads. Plain cards, one per topic, so it reads top to bottom.
const sections: { title: string; body: (string | string[])[] }[] = [
  {
    title: "Connaio 是什么",
    body: [
      "一个把网络的几个方向放在一起的工具：出站代理、进入别处网络的 VPN、把本机暴露出去的反向隧道、给局域网设备共享的代理，以及接管本机流量的几种方式。内核是 mihomo 的分支，界面里的每样东西在内核里都归结为两种原语：节点和规则。",
      "典型用法：订阅里的节点按规则分流；校园网或公司网用 VPN 页连上，校内地址自动走隧道；反向页把本机服务经 FRP 或 SSH 暴露出去；设备页让手机、平板用这台电脑的代理。",
    ],
  },
  {
    title: "接管流量的三种方式",
    body: [
      "系统代理：把系统的 HTTP / SOCKS 代理指向本机 7899 端口。浏览器和多数桌面应用会跟随；命令行工具（ssh、git、curl）不会。不需要管理员权限。",
      "增强模式（TUN）：接管全部流量，包括不认系统代理的程序。需要先在「通用」里授予一次管理员权限，之后内核作为系统服务运行。",
      "部分增强：只接管勾选的应用和它们的子进程，其余照常。列表里有正在运行的应用，也有常用命令行工具（ssh、git、curl、pip、npm 等）可以直接勾。",
      "退出应用时这些都会撤掉：系统代理清除、TUN 关闭、VPN 登出、反向隧道关闭。应用崩溃时内核也会自己退出。",
    ],
  },
  {
    title: "配置与覆写",
    body: [
      "「配置」页管理订阅和本地配置文件，选中的一份作为当前配置。订阅会定期更新。",
      "你在界面里添加的节点、策略组、规则、隧道、VPN 都写进「覆写」（profiles/override.yaml），叠加在当前配置之上，订阅更新不会冲掉它们。覆写也可以手动编辑。",
      "内核读到的最终配置 = 应用默认值 + 当前配置 + 覆写。默认值：mixed-port 7899、rule 模式、按进程统计流量、capture 为系统代理。",
    ],
  },
  {
    title: "策略与规则",
    body: [
      "「策略」页列出节点和策略组，可以测延迟、切换当前节点、添加或编辑节点。添加节点对话框里的「前置代理」能让一个节点经另一个节点连出去，例如只有校内能访问的代理经 VPN 连。",
      "「规则」页从上到下匹配，第一条命中的规则决定走哪个节点。自己加的规则排在最前，可以上下移动；订阅里的规则可以隐藏。",
      "策略组类型里的 Subnet（按网络自动选择）会按当前 Wi-Fi 名称、BSSID、网关地址、本机网段或连接方式选成员，不需要手动切换。",
    ],
  },
  {
    title: "VPN",
    body: [
      "支持 EasyConnect（深信服 SSL VPN）、WireGuard、OpenVPN（粘贴 .ovpn 文件内容）和 Tailscale。每个 VPN 是一个节点加几条规则，连上以后在策略页和规则页都能看到。",
      "EasyConnect 登录后网关会下发它路由的网段和域名，这些自动走隧道（规则页里的 EASYCONNECT 规则），其余流量不受影响。「测试登录」用当前填写的账号真实登录一次并立刻登出，不保存。",
      "附加路由：网关列表之外还要走 VPN 的网段、域名、关键字或 GEOSITE / GEOIP。",
      "直连网络：在这些网络里不走 VPN，比如已经在校内时。按 Wi-Fi 名称、BSSID、网关地址、本机网段，或任何 Wi-Fi / 有线匹配；「当前网络」一键填入。有条件时会生成一个按网络选择的策略组。",
      "「断开」只撤掉配置、保留设置，「删除」整个去掉。状态栏菜单里每个 VPN 一项，点一下切换。",
      "列表下面的「测试」填一个目标地址，经本机代理端口真连一次，报告命中的规则和出口节点。",
      "注意：命令行的 ssh 不走系统代理，要经 VPN 连校内服务器请开增强模式或部分增强，或用 ProxyCommand 走本机代理端口。",
    ],
  },
  {
    title: "反向隧道",
    body: [
      "「反向」页通过 FRP 或 SSH 把本机的服务（默认是本机代理端口）暴露到远端服务器，外面的设备连那台服务器就能用这台电脑的代理，进来的连接按规则分流。",
      "服务器在 VPN 后面时，隧道的「跳板代理」选对应的 VPN。可以用「部署 frps」一键在你的服务器上装好 FRP 服务端。",
    ],
  },
  {
    title: "设备",
    body: [
      "打开「允许局域网」后，同一网络里的设备把代理填成这台电脑的地址和 7899 端口就能用。设备页按来源地址列出它们，并可以把某台设备固定到某个策略。",
    ],
  },
  {
    title: "活动、连接、诊断与修复",
    body: [
      "「活动」页看实时速度、今日和本月流量按进程、主机和策略的分布，以及网络延迟。「连接」页列出每条连接的进程、目标、命中的规则和出口，可以按主机或来源关闭，也能加临时规则。",
      "「网络诊断」逐项检查配置、本机网络、路由、DNS、直连和代理。「网络修复」清理代理程序退出后留下的残留：指向已退出程序的系统代理、残留的内核进程、残留的 TUN 接口，另有 DNS 缓存刷新，Windows 上还有 Winsock 和 TCP/IP 重置。",
    ],
  },
  {
    title: `${words.tray}与通用`,
    body: [
      `${words.tray}图标显示当前速度和是否在接管流量，菜单里可以切出站模式、选节点、开关系统代理和增强模式、连接 VPN，并看到最近有流量的进程。`,
      "「通用」页：代理端口、日志级别、重启内核、授予或移除管理员权限、开机启动、检查更新。更新从 GitHub Releases 下载并校验。",
    ],
  },
  {
    title: "内核读取的配置要点",
    body: [
      "下面这些键由内核直接读取，写在覆写里即可：",
      [
        "capture：接管方式，mode 为 system-proxy、tun 或 process-tun，process-tun 时 processes 列出进程名。",
        "reverse-tunnels：反向隧道列表，type 为 frp 或 ssh，可带 dialer-proxy。",
        "proxies 里 type: easyconnect 的节点：server、port、username、password、totp-secret（可选）、udp。",
        "规则 EASYCONNECT,<节点>,<策略>：匹配该节点的网关下发的网段和域名。",
        "proxy-groups 里 type: subnet 的组：default 和 rules（SSID:名称、BSSID:地址、ROUTER:网关、网段、TYPE:WIFI、TYPE:WIRED → 成员）。",
        "allow-lan：允许局域网设备使用代理端口。",
      ],
      "其余键与 mihomo 相同。external-controller 之类的控制键会被忽略，界面通过自己的通道连内核。",
    ],
  },
  {
    title: "数据存放位置",
    body: [
      "macOS：~/Library/Application Support/Connaio；Linux：~/.config/Connaio；Windows：%AppData%\\Connaio。里面有 config.yaml（内核读的最终配置）、core.log、traffic.json、settings.json 和 profiles/。",
      "管理员权限下运行的内核数据在系统目录：macOS 为 /Library/Application Support/Connaio Core，Linux 为 /var/lib/connaio，Windows 为 %ProgramData%\\Connaio Core。",
    ],
  },
  {
    title: "常见问题",
    body: [
      [
        "浏览器能上网但 ssh 连不上校内：ssh 不走系统代理，开增强模式或部分增强。",
        "VPN 登录被拒绝「the gateway refused a tunnel address」：同一账号已有会话，多半是上一个会话还没过期，等几分钟再连；别的设备登着同一账号也会这样。",
        "密码错误等明确拒绝不会自动重试，改好设置再点连接；其他失败 30 秒后自动重试。",
        "退出后网络不通：打开应用，在「活动」页点「网络修复」，清掉指向已退出程序的系统代理。",
        "Windows 上增强模式要装系统服务，卸载应用时会一并删除。",
      ],
    ],
  },
];

export default function Guide() {
  return (
    <>
      {sections.map((s) => (
        <div key={s.title} className="card" style={{ marginBottom: 16 }}>
          <div className="label">{s.title}</div>
          {s.body.map((b, i) => Array.isArray(b)
            ? <ul key={i} className="small" style={{ margin: "8px 0 0 18px", lineHeight: 1.7, color: "var(--text)" }}>{b.map((x) => <li key={x}>{x}</li>)}</ul>
            : <p key={i} className="small" style={{ margin: "8px 0 0", lineHeight: 1.7 }}>{b}</p>)}
        </div>
      ))}
    </>
  );
}
