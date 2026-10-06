"""Reviewed, fail-closed renderer patches for official desktop build 13232."""
from pathlib import Path
import re


def replace(text: str, anchor: str, replacement: str) -> str:
    count = text.count(anchor)
    if count != 1:
        raise RuntimeError(f"build 13232: expected one anchor {anchor[:90]!r}, found {count}")
    return text.replace(anchor, replacement, 1)


def patch_renderer(extracted: Path, token: str, project: Path, port: int) -> None:
    assets = extracted / 'webview' / 'assets'
    initial_path = assets / 'app-initial-69cd8dbddec5.js'
    shared_path = assets / 'app-shared-122c56612a72.js'
    menu_path = assets / 'profile-dropdown-items-8de2bbe3af44.js'
    modal_path = assets / 'modal-impl-ae22cdc34021.js'
    plugins_path = assets / 'plugins-settings-9d5c3f603c0d.js'
    initial, shared, menu, modal, plugins = [p.read_text() for p in
        (initial_path, shared_path, menu_path, modal_path, plugins_path)]
    index_path = extracted / 'webview' / 'index.html'
    index = replace(index_path.read_text(), "connect-src &#39;self&#39;",
                    f"connect-src &#39;self&#39; http://127.0.0.1:{port}")
    thread_hook = 'function oW(){let e=(0,cai.c)(2),t=Dm(`/local/:conversationId`)'
    if initial.count(thread_hook) != 1:
        raise RuntimeError('build 13232: native conversation hook changed')

    component = (project / 'ui' / 'account-menu.js').read_text()
    component = component.replace('__CODEX_MUX_CONTROL_PORT__', str(port))
    component = component.replace('__CODEX_MUX_CONTROL_TOKEN__', token)
    bindings = {'e7': 'CodexMuxJsx', 'kXc': 'CodexMuxReact', '_H': 'Cke',
                'CH': 'wu', 'Lo': 'pp', 'Q': 'W', 'BW': 'gs',
                'QLs': 'CodexMuxNativeResetModal', 'S2': 'cse',
                'lt': 'Xl',
                'jLa': 'CodexMuxResolveAvatar'}
    component = re.sub(r'\b[A-Za-z_$][\w$]*\b',
                       lambda m: bindings.get(m[0], m[0]), component)
    # The account UI lives in the eager bundle so footer state is ready before
    # the native dropdown's lazy module is opened for the first time.
    bridge = '''
const CodexMuxReact=Nu(),CodexMuxJsx=X();
globalThis.codexMuxNativeThreadIdHook=()=>{sW();return oW()};
globalThis.codexMuxNativeRateLimitResets=Asr;
globalThis.codexMuxNativeConsumeRateLimitReset=Msr;
function CodexMuxNativeResetModal(props){fRi();return CodexMuxJsx.jsx(sRi,props)}
function CodexMuxResolveAvatar(url){if(!url)return null;try{let u=new URL(url);return u.protocol===`https:`||url.startsWith(`data:image/`)?url:null}catch{return null}}
'''
    initial = replace(initial, 'function Clo(e){let t=(0,Tlo.c)(40),',
                      bridge + component + '\nfunction Clo(e){let t=(0,Tlo.c)(40),')
    # Each native open-state path uses the same guarded handler.
    for anchor in ['triggerButton:w,onOpenChange:b,children:D',
                   'open:c,onOpenChange:b,contentWidth:`panel`,triggerButton:w']:
        initial = replace(initial, anchor, anchor.replace('onOpenChange:b',
                          'onOpenChange:CodexMuxProfileMenuOpenChange(b)'))
    initial = replace(initial, 'onClick:Qe,children:[qe,$e,et]',
                      'onClick:Qe,children:[qe,(0,u$.jsx)(globalThis.CodexMuxAccountStatus,{compact:i})]')
    initial = replace(initial, 'if(o&&E&&p===`chatgpt`&&!i&&k?.id===d',
                      'if(false&&o&&E&&p===`chatgpt`&&!i&&k?.id===d')
    # Replace the native single-account quota slot in both menu layouts.
    menu = replace(menu, ':null;if(r!=null){let e=r.profileIdentity',
                   ':null;qn=(0,$.jsx)(globalThis.CodexMuxAccountMenu,{});if(r!=null){let e=r.profileIdentity')
    menu = replace(menu, 'accountIcon:o,accountSwitcher:Hn,additionalItems:g',
                   'accountIcon:o,accountSwitcher:null,additionalItems:g')
    # Export the injected component without changing upstream module exports.
    initial += '\n;globalThis.CodexMuxAccountMenu=CodexMuxAccountMenu;\n'

    rpc = 'async sendRequest(e,t,n){if(this.dispatchMessage==null)throw Error(`AppServerRequestClient is missing a message dispatcher`);'
    shared = replace(shared, rpc, rpc + 't=globalThis.codexMuxScopePluginRequest?.(e,t)??t;')
    for method in ('app/list', 'app/installed', 'app/read', 'mcpServer/oauth/login', 'mcpServerStatus/list'):
        if f'`{method}`' not in initial + shared:
            raise RuntimeError(f'build 13232: missing native plugin RPC {method}')
    initial = replace(initial, 'let e=await cu.safeGet(`/wham/profiles/me`)',
                      'let e=await globalThis.codexMuxProfileData(globalThis.__codexMuxSelectedProfileAccountId??null)')
    query = 'function Osr(){let e=(0,wL.c)(1);Ei(),$(null);let t;return e[0]===Symbol.for(`react.memo_cache_sentinel`)?(t={queryKey:[`rate-limit-reset-credits`],queryFn:Asr,select:ksr,refetchInterval:rl.ONE_MINUTE,staleTime:rl.FIVE_SECONDS},e[0]=t):t=e[0],Qu(t)}'
    initial = replace(initial, query,
        'function Osr(){Ei(),$(null);let e=window.__codexMuxResetAccountId;return Qu({queryKey:[`rate-limit-reset-credits`,e??`primary`],queryFn:e?()=>globalThis.codexMuxRateLimitResets(e):Asr,select:ksr,refetchInterval:rl.ONE_MINUTE,staleTime:rl.FIVE_SECONDS})}')
    mutation = 'function jsr(){let e=(0,wL.c)(3),t=Xl(),n=bf(),r;return e[0]!==n||e[1]!==t?(r={mutationFn:Msr,onSuccess:(e,r)=>{let{creditId:i}=r,a=e.code;if(a===`reset`||a===`already_redeemed`){let n=e.code===`reset`?e.credit?.id??i:i;t.setQueryData([`rate-limit-reset-credits`],e=>rsr(e,a,n))}Promise.all([n([`rate-limit-status`]),n([`rate-limit-reset-credits`])])}},e[0]=n,e[1]=t,e[2]=r):r=e[2],es(r)}'
    initial = replace(initial, mutation,
        'function jsr(){let e=Xl(),t=bf(),n=window.__codexMuxResetAccountId,r=[`rate-limit-reset-credits`,n??`primary`];return es({mutationFn:n?i=>globalThis.codexMuxConsumeRateLimitReset(n,i):Msr,onSuccess:(n,i)=>{let{creditId:a}=i,o=n.code;if(o===`reset`||o===`already_redeemed`){let t=o===`reset`?n.credit?.id??a:a;e.setQueryData(r,e=>rsr(e,o,t))}Promise.all([t([`rate-limit-status`]),t(r)])}})}')
    # Wrap the lazy reset dialog so its query children render after account state.
    initial = replace(initial, 'function sRi(e){let t=(0,lRi.c)(7),',
                      'function sRi(e){CodexMuxUseResetAccountState();let t=(0,lRi.c)(7),')
    modal = replace(modal, 'let ee=U,te;',
                    'let ee=window.__codexMuxSelectedUsageWindows??U,te;')
    modal = replace(modal, 'children:[je,Le,Re,ze]',
                    'children:[je,window.__codexMuxResetAccountSelector??null,Le,Re,ze]')
    plugins = replace(plugins, 'title:h,subtitle:g,action:S,children:m}',
                      'title:h,subtitle:g,action:S,children:[globalThis.CodexMuxPluginScope?.()??null,m]}')

    for path, text in [(index_path, index), (initial_path, initial),
                       (shared_path, shared), (menu_path, menu),
                       (modal_path, modal), (plugins_path, plugins)]:
        path.write_text(text)
