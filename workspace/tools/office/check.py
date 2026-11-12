"""Check every dependency used by the bundled office skills."""
import importlib
import shutil
import subprocess
import sys

for module in ("openpyxl", "pandas", "markitdown", "PIL", "defusedxml", "lxml", "pptx"):
    importlib.import_module(module)
    print(f"OK Python: {module}")
for command in ("node", "pandoc", "soffice", "pdftoppm", "pdfinfo", "zip", "unzip", "gcc"):
    if not shutil.which(command):
        sys.exit(f"Missing command: {command}")
    print(f"OK command: {command}")
subprocess.run(["node", "-e", "for (const m of ['docx','pptxgenjs','react','react-dom/server','react-icons/fa','sharp']) { require(m); console.log('OK Node: '+m) }"], check=True)
fonts = subprocess.check_output(["fc-list", ":lang=zh", "family"], text=True).strip()
if not fonts:
    sys.exit("Missing Chinese fonts")
print("OK Chinese fonts: " + fonts.splitlines()[0])
print("Office runtime ready")
