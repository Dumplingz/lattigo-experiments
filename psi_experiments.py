import subprocess
import time
import os
import csv

num_trials = 10
data_sizes = ["1MB", "10MB", "100MB", "1GB", "10GB"]
data_size_dict = {"1MB": [10,12289], "10MB": [13,65537], "100MB": [17,786433], "1GB": [20,23068673], "10GB": [23,167772161]}

if __name__ == "__main__":

    # get path of current directory
    dir_path = os.path.dirname(os.path.realpath(__file__))
    print(dir_path)
    # make experiments directory
    os.makedirs(dir_path + "/experiments", exist_ok=True)

    for data_size in data_sizes:
        time_start = time.perf_counter()
        # go run psi/main.go num_parties goroutines logN num_MB
        result = subprocess.run(["go", "run", "psi/main.go",str(2),str(1),
                                str(data_size_dict[data_size][0]),
                                str(data_size_dict[data_size][1]),
                                data_size]) 
        time_end = time.perf_counter()

        print(result.stdout)
        total_time = time_end - time_start
        print(f"Time taken: {total_time}")
        with open(f"experiments/{data_size}.csv", "a") as file:
            writer = csv.writer(file)
            writer.writerow([total_time])