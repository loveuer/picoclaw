"""Exercise office read/write, recalculation, rendering, and Chinese paths.

Run inside the office runtime. Writes fixtures into the directory passed as argv[1].
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import zipfile

from PIL import Image
from openpyxl import Workbook, load_workbook

runtime = Path(__file__).resolve().parent
workspace = runtime.parent.parent
out = Path(sys.argv[1]).resolve()
out.mkdir(parents=True, exist_ok=False)
os.chdir(out)


def run(*args):
    try:
        return subprocess.check_output([str(a) for a in args], text=True, stderr=subprocess.STDOUT, timeout=120)
    except subprocess.CalledProcessError as error:
        print(error.output, file=sys.stderr)
        raise


def helper(skill, script, *args):
    return run(sys.executable, workspace / 'skills' / skill / 'scripts' / script, *args)


def replace_xml(source, target, member, before, after):
    with zipfile.ZipFile(source) as src, zipfile.ZipFile(target, 'w', zipfile.ZIP_DEFLATED) as dst:
        found = False
        for info in src.infolist():
            data = src.read(info.filename)
            if info.filename == member:
                text = data.decode()
                assert before in text, (source, member)
                data = text.replace(before, after).encode()
                found = True
            dst.writestr(info, data)
        assert found


Image.new('RGB', (80, 40), '#008080').save('example.png')
Path('generate.cjs').write_text(r'''
const fs = require('fs');
const {Document,Packer,Paragraph,Table,TableRow,TableCell,ImageRun,WidthType} = require('docx');
const PptxGenJS = require('pptxgenjs');
(async () => {
 const doc = new Document({styles: {default: {document: {run: {font:'Noto Sans CJK SC'}}}}, sections: [{children: [
  new Paragraph('办公测试：原始报告'),
  new Table({width:{size:8000,type:WidthType.DXA}, columnWidths:[4000,4000], rows:[new TableRow({children:[
   new TableCell({width:{size:4000,type:WidthType.DXA},children:[new Paragraph('项目')]}),
   new TableCell({width:{size:4000,type:WidthType.DXA},children:[new Paragraph('金额')]}),
  ]})]}),
  new Paragraph({children:[new ImageRun({type:'png',data:fs.readFileSync('example.png'),transformation:{width:80,height:40}})]})
 ]}]});
 fs.writeFileSync('原始 报告.docx',await Packer.toBuffer(doc));
 const ppt = new PptxGenJS(); ppt.layout='LAYOUT_WIDE';
 let s=ppt.addSlide();s.addText('办公测试：原始演示',{x:1,y:1,w:11,h:1,fontFace:'Noto Sans CJK SC',fontSize:32});
 s.addImage({path:'example.png',x:1,y:3,w:2,h:1});
 s=ppt.addSlide();s.addText('第二页：数据说明',{x:1,y:1,w:11,h:1,fontFace:'Noto Sans CJK SC',fontSize:28});
 await ppt.writeFile({fileName:'原始 演示.pptx'});
})();
''', encoding='utf-8')
run('node', 'generate.cjs')
assert '原始报告' in run('pandoc', '-t', 'plain', '原始 报告.docx')
replace_xml('原始 报告.docx', '修改 报告.docx', 'word/document.xml', '原始报告', '修改后的报告')
assert '修改后的报告' in run('pandoc', '-t', 'plain', '修改 报告.docx')
helper('docx', 'merge_runs.py', '修改 报告.docx', '-o', '合并 报告.docx')
helper('docx', 'comment.py', '合并 报告.docx', '请核对金额', '-o', '批注 报告.docx')
with zipfile.ZipFile('批注 报告.docx') as z:
    assert '请核对金额' in z.read('word/comments.xml').decode()
helper('docx', 'office/validate.py', '修改 报告.docx')

book = Workbook()
sheet = book.active
sheet.title = '费用'
sheet.append(['项目', '金额'])
sheet.append(['咖啡', 12])
sheet.append(['午餐', 30])
sheet.append(['合计', '=SUM(B2:B3)'])
book.save('原始 费用.xlsx')
report = json.loads(helper('xlsx', 'recalc.py', '原始 费用.xlsx', '60'))
assert report.get('status') == 'success', report
assert load_workbook('原始 费用.xlsx', data_only=True).active['B4'].value == 42
book = load_workbook('原始 费用.xlsx')
book.active['B2'] = 18
book.save('修改 费用.xlsx')
report = json.loads(helper('xlsx', 'recalc.py', '修改 费用.xlsx', '60'))
assert report.get('status') == 'success', report
assert load_workbook('修改 费用.xlsx', data_only=True).active['B4'].value == 48
assert load_workbook('修改 费用.xlsx').active['B4'].value.startswith('=SUM(')

assert '原始演示' in run('markitdown', '原始 演示.pptx')
replace_xml('原始 演示.pptx', '修改 演示.pptx', 'ppt/slides/slide1.xml', '原始演示', '修改后的演示')
text = run('markitdown', '修改 演示.pptx')
assert '修改后的演示' in text and '第二页' in text
helper('pptx', 'office/validate.py', '修改 演示.pptx')
helper('pptx', 'thumbnail.py', '修改 演示.pptx', '演示缩略图')

for skill, filename in [('docx', '修改 报告.docx'), ('pptx', '修改 演示.pptx')]:
    helper(skill, 'office/soffice.py', '--headless', '--convert-to', 'pdf', filename, '--outdir', str(out))
    pdf = Path(filename).with_suffix('.pdf')
    assert pdf.exists() and pdf.stat().st_size > 0
    assert '修改后' in run('pdftotext', str(pdf), '-')
    run('pdftoppm', '-f', '1', '-singlefile', '-scale-to', '1200', '-png', str(pdf), str(pdf.with_suffix('')))
    if skill == 'pptx':
        assert 'Pages:           2' in run('pdfinfo', str(pdf))

assert Path('修改 报告.docx').stat().st_uid == os.getuid()
print(json.dumps({'status':'success','xlsx_totals':[42,48], 'pptx_pages':2,'output':str(out)},ensure_ascii=False))
