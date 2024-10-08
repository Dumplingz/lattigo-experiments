import subprocess
import time
import os 

dir_path = os.path.dirname(os.path.realpath(__file__))
print(dir_path)

time_start = time.perf_counter()
result = subprocess.run(["go", "run", "examples/multiparty/int_psi/main.go"]) 
time_end = time.perf_counter()

print(result.stdout)
print(time_end - time_start)