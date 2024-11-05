import subprocess
import time
import os

data_size = "10GB"

data_size_dict = {"1MB": [10,12289], "10MB": [13,65537], "100MB": [17,786433], "1GB": [20,23068673], "10GB": [23,167772161]}

dir_path = os.path.dirname(os.path.realpath(__file__))
print(dir_path)

time_start = time.perf_counter()
# go run psi/main.go num_parties goroutines logN num_MB
result = subprocess.run(["go", "run", "psi/main.go",str(2),str(1),
                         str(data_size_dict[data_size][0]),
                         str(data_size_dict[data_size][1]),
                         data_size]) 
time_end = time.perf_counter()

print(result.stdout)
print(time_end - time_start)