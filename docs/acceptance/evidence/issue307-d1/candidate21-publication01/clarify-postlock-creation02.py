"""Preserve v01; clarify the observed creation of a previously absent rule."""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[5]
HERE = Path(__file__).parent
DOCS = ROOT / 'docs/acceptance'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def write(path, text):
    with path.open('xb') as stream:
        stream.write(text.encode('utf-8'))


def main():
    binding = json.loads((HERE / 'branch-freeze01/execution-binding.json').read_bytes())
    lock = binding['branch_lock']
    if lock['before']['rule'] is not None or lock['before']['protection'] is not None:
        raise ValueError('Clarification requires actual native absence before creation')
    old = DOCS / 'issue307-d1-candidate21-final-confirmation01.md'
    text = old.read_text(encoding='utf-8')
    phrase = '非freeze保护与锁前一致，锁后不得推进HEAD'
    if text.count(phrase) != 1:
        raise ValueError('Original v01 rendered claim differs')
    text = text.replace(phrase,
        '锁前原生rule/protection均经确认不存在；本次新建exact规则使分支从无保护收紧，不声称存在旧非freeze字典或已作逐字段相等比较。锁后不得推进HEAD')
    text += '\n\n本02文本仅精确化原生规则新建事实，保留01文本/checkpoint/核对回执原字节作为历史。execution、候选、模板、全部运行绑定、窗口、最终HEAD及execution-binding均不变；本次未新增授权或发送。\n'
    current = DOCS / 'issue307-d1-candidate21-final-confirmation02.md'
    write(current, text)
    prior = json.loads((HERE / 'final-checkpoint01.json').read_bytes())
    record = dict(prior)
    record.update(final_proposal=str(current.relative_to(ROOT)), final_proposal_sha256=sha(current),
        preceding_checkpoint_sha256=sha(HERE/'final-checkpoint01.json'),
        preserved_proposal01_sha256=sha(old), branch_operation='createBranchProtectionRule',
        prior_rule=None, prior_native_protection=None,
        non_freeze_policy_equality='NOT_APPLICABLE_NO_PRIOR_PROTECTION',
        clarification='Rule creation tightens from confirmed native absence; runtime/artifact/window/HEAD/binding unchanged')
    write(HERE / 'final-checkpoint02.json', json.dumps(record, ensure_ascii=False, indent=2)+'\n')
    original_matrix = DOCS / 'issue307-d1-v31-postlock-matrix01.md'
    matrix = original_matrix.read_text(encoding='utf-8').replace('issue307-d1-candidate21-final-confirmation01.md',
        'issue307-d1-candidate21-final-confirmation02.md')
    matrix += '\n\n原生冻结操作为createBranchProtectionRule；锁前rule/protection均已确认不存在。当前新exact规则从无保护收紧，不把未发生的旧非freeze字典比较写作PASS。01矩阵/方案/回执不覆盖，本02仅追加事实精确化。\n'
    write(DOCS / 'issue307-d1-v31-postlock-matrix02.md', matrix)
    print(json.dumps(dict(status='PASS_POSTLOCK_RULE_CREATION_FACT_CLARIFIED_APPEND_ONLY',
        proposal_sha256=sha(current), final_head=record['final_head'],
        execution_binding_sha256=record['execution_binding_sha256'], native_delete=0)))


if __name__ == '__main__':
    main()
